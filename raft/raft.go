package raft

import (
	"bright/store"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"go.uber.org/zap"
)

// RaftNode represents a Raft consensus node
type RaftNode struct {
	raft      *raft.Raft
	fsm       *FSM
	config    *RaftConfig
	transport *raft.NetworkTransport
	logger    *zap.Logger
}

// RaftConfig contains configuration for initializing a Raft node
type RaftConfig struct {
	NodeID       string   // Unique node identifier (e.g., "node-0")
	RaftDir      string   // Directory for Raft persistent state
	RaftBind     string   // Address for Raft transport (e.g., "0.0.0.0:7000")
	RaftAdvertise string  // Advertisable address for Raft (e.g., "node-0.bright:7000")
	Bootstrap    bool     // Is this the initial cluster bootstrap node?
	Peers        []string // Initial peer addresses (e.g., ["node-0.bright:7000"])
}

// NewRaftNode creates and initializes a new Raft node
func NewRaftNode(config *RaftConfig, indexStore *store.IndexStore, logger *zap.Logger) (*RaftNode, error) {
	// Create Raft configuration
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.NodeID)
	raftConfig.SnapshotThreshold = 1024 // Snapshot after 1024 log entries

	// Setup FSM
	fsm := NewFSM(indexStore)

	// Setup persistent stores
	if err := os.MkdirAll(config.RaftDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create raft directory: %w", err)
	}

	// BoltDB for log storage
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(config.RaftDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create log store: %w", err)
	}

	// BoltDB for stable storage
	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(config.RaftDir, "raft-stable.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create stable store: %w", err)
	}

	// File-based snapshot store (keeps last 3 snapshots)
	snapshotStore, err := raft.NewFileSnapshotStore(config.RaftDir, 3, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot store: %w", err)
	}

	// Setup network transport
	// Use advertise address if provided, otherwise use bind address
	advertiseAddr := config.RaftAdvertise
	if advertiseAddr == "" {
		advertiseAddr = config.RaftBind
	}

	advAddr, err := net.ResolveTCPAddr("tcp", advertiseAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve advertise address: %w", err)
	}

	transport, err := raft.NewTCPTransport(config.RaftBind, advAddr, 3, 10*time.Second, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	// Create Raft node
	raftNode, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft node: %w", err)
	}

	// Bootstrap cluster if this is the first node
	if config.Bootstrap {
		// Use advertise address for stable DNS-based addressing
		bootstrapAddr := raft.ServerAddress(advertiseAddr)

		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: bootstrapAddr,
				},
			},
		}
		raftNode.BootstrapCluster(configuration)
	} else if len(config.Peers) > 0 {
		// Non-bootstrap nodes: attempt to auto-join the cluster
		// This happens in the background after startup
		go func() {
			// Wait for the transport to be fully ready
			time.Sleep(3 * time.Second)

			logger.Info("Raft node starting",
				zap.String("node_id", config.NodeID),
				zap.String("listen_addr", string(transport.LocalAddr())),
				zap.String("advertise_addr", advertiseAddr),
			)

			// Try contacting peers to join the cluster
			maxRetries := 30
			retryDelay := 5 * time.Second

			for attempt := 0; attempt < maxRetries; attempt++ {
				for _, peerAddr := range config.Peers {
					// Skip self
					if peerAddr == advertiseAddr {
						continue
					}

					logger.Info("Attempting to join cluster",
						zap.String("peer", peerAddr),
						zap.Int("attempt", attempt+1),
						zap.Int("max_retries", maxRetries),
					)

					// Convert Raft address (host:7000) to HTTP API address (host:3000)
					httpAddr := strings.Replace(peerAddr, ":7000", ":3000", 1)

					// Prepare join request with stable DNS-based address
					joinReq := map[string]string{
						"node_id": config.NodeID,
						"addr":    advertiseAddr, // Use DNS name instead of IP
					}

					jsonData, err := json.Marshal(joinReq)
					if err != nil {
						logger.Error("Failed to marshal join request", zap.Error(err))
						continue
					}

					// Send HTTP POST to /cluster/join
					httpClient := &http.Client{Timeout: 5 * time.Second}
					resp, err := httpClient.Post(
						fmt.Sprintf("http://%s/cluster/join", httpAddr),
						"application/json",
						bytes.NewBuffer(jsonData),
					)

					if err != nil {
						logger.Warn("Failed to contact peer",
							zap.String("peer", httpAddr),
							zap.Error(err),
						)
						continue
					}

					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()

					if resp.StatusCode == http.StatusOK {
						logger.Info("Successfully joined cluster",
							zap.String("peer", httpAddr),
							zap.String("node_id", config.NodeID),
						)
						return
					} else {
						logger.Warn("Join request failed",
							zap.String("peer", httpAddr),
							zap.Int("status", resp.StatusCode),
							zap.String("response", string(body)),
						)
					}
				}

				// Wait before retrying
				if attempt < maxRetries-1 {
					time.Sleep(retryDelay)
				}
			}

			logger.Error("Failed to auto-join cluster",
				zap.Int("attempts", maxRetries),
				zap.String("node_id", config.NodeID),
			)
		}()
	}

	return &RaftNode{
		raft:      raftNode,
		fsm:       fsm,
		config:    config,
		transport: transport,
		logger:    logger,
	}, nil
}

// IsLeader returns true if this node is the current Raft leader
func (r *RaftNode) IsLeader() bool {
	return r.raft.State() == raft.Leader
}

// LeaderAddr returns the address of the current leader
func (r *RaftNode) LeaderAddr() string {
	_, leaderID := r.raft.LeaderWithID()
	return string(leaderID)
}

// Apply submits a command to the Raft log for replication
func (r *RaftNode) Apply(cmd Command, timeout time.Duration) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	future := r.raft.Apply(data, timeout)
	if err := future.Error(); err != nil {
		return err
	}

	// Check if the command application returned an error
	if result := future.Response(); result != nil {
		if err, ok := result.(error); ok {
			return err
		}
	}

	return nil
}

// Join adds a new node to the Raft cluster
func (r *RaftNode) Join(nodeID, addr string) error {
	future := r.raft.AddVoter(raft.ServerID(nodeID), raft.ServerAddress(addr), 0, 0)
	return future.Error()
}

// Shutdown gracefully shuts down the Raft node
func (r *RaftNode) Shutdown() error {
	return r.raft.Shutdown().Error()
}

// GetConfig returns the Raft configuration
func (r *RaftNode) GetConfig() *RaftConfig {
	return r.config
}

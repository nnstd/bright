package raft

import (
	"bright/store"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
)

// RaftNode represents a Raft consensus node
type RaftNode struct {
	raft      *raft.Raft
	fsm       *FSM
	config    *RaftConfig
	transport *raft.NetworkTransport
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
func NewRaftNode(config *RaftConfig, indexStore *store.IndexStore) (*RaftNode, error) {
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
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raftConfig.LocalID,
					Address: transport.LocalAddr(),
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

			// Try contacting peers to join the cluster
			for _, peerAddr := range config.Peers {
				// Skip self
				if peerAddr == advertiseAddr {
					continue
				}

				fmt.Fprintf(os.Stderr, "[RAFT] Attempting auto-join to cluster via peer: %s\n", peerAddr)

				// Contact the peer's Raft transport to request joining
				// The leader should add us via the AddVoter call
				// For now, we rely on manual join via API or the leader detecting us
				// In a production setup, you'd implement a discovery/join protocol
			}

			fmt.Fprintf(os.Stderr, "[RAFT] Waiting for leader to add this node to the cluster...\n")
			fmt.Fprintf(os.Stderr, "[RAFT] Node %s listening at %s\n", config.NodeID, transport.LocalAddr())
		}()
	}

	return &RaftNode{
		raft:      raftNode,
		fsm:       fsm,
		config:    config,
		transport: transport,
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

package config

import (
	"strings"

	"github.com/caarlos0/env/v11"
)

// Config holds the application configuration
type Config struct {
	Port      string `env:"PORT,HTTP_PORT,BRIGHT_PORT" envDefault:"3000"`
	MasterKey string `env:"MASTER_KEY,BRIGHT_MASTER_KEY"`
	LogLevel  string `env:"LOG_LEVEL,BRIGHT_LOG_LEVEL" envDefault:"info"`
	DataPath  string `env:"DATA_PATH,BRIGHT_DATA_PATH" envDefault:"./data"`

	// Auto-create indexes on first document insert
	AutoCreateIndex bool `env:"AUTO_CREATE_INDEX,BRIGHT_AUTO_CREATE_INDEX" envDefault:"true"`

	// Raft configuration
	RaftEnabled   bool   `env:"RAFT_ENABLED" envDefault:"false"`
	RaftNodeID    string `env:"RAFT_NODE_ID"`
	RaftDir       string `env:"RAFT_DIR"`
	RaftBind      string `env:"RAFT_BIND" envDefault:"0.0.0.0:7000"`
	RaftAdvertise string `env:"RAFT_ADVERTISE"` // Advertisable address for Raft
	RaftBootstrap bool   `env:"RAFT_BOOTSTRAP" envDefault:"false"`
	RaftPeers     string `env:"RAFT_PEERS"` // Comma-separated peer addresses
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	// Set RaftDir to DataPath/raft if not explicitly configured
	if cfg.RaftDir == "" {
		cfg.RaftDir = cfg.DataPath + "/raft"
	}

	// Auto-detect bootstrap for StatefulSet pod-0
	// If RAFT_BOOTSTRAP env var is not explicitly set and Raft is enabled,
	// enable bootstrap for the first pod (node ID ending with -0)
	if cfg.RaftEnabled && !cfg.RaftBootstrap {
		if strings.HasSuffix(cfg.RaftNodeID, "-0") {
			cfg.RaftBootstrap = true
		}
	}

	// Validate RaftAdvertise is set when Raft is enabled
	// This is critical for stable addressing in Kubernetes
	if cfg.RaftEnabled && cfg.RaftAdvertise == "" {
		// Fallback to RaftBind if not set (for backward compatibility)
		// but this is not recommended in production
		cfg.RaftAdvertise = cfg.RaftBind
	}

	return cfg, nil
}

// RequiresAuth returns true if authentication is enabled
func (c *Config) RequiresAuth() bool {
	return c.MasterKey != ""
}

// GetRaftPeers parses the comma-separated RAFT_PEERS environment variable
func (c *Config) GetRaftPeers() []string {
	if c.RaftPeers == "" {
		return []string{}
	}
	peers := strings.Split(c.RaftPeers, ",")
	// Trim whitespace from each peer
	for i := range peers {
		peers[i] = strings.TrimSpace(peers[i])
	}
	return peers
}

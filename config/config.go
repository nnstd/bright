package config

import (
	"os"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Config holds the application configuration
type Config struct {
	Port      string `env:"BRIGHT_PORT" envDefault:"3000"`
	MasterKey string `env:"BRIGHT_MASTER_KEY"`
	LogLevel  string `env:"BRIGHT_LOG_LEVEL" envDefault:"info"`
	DataPath  string `env:"BRIGHT_DATA_PATH" envDefault:"./data"`

	// Auto-create indexes on first document insert
	AutoCreateIndex bool `env:"BRIGHT_AUTO_CREATE_INDEX" envDefault:"true"`

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

	// Support environment variable aliases for backward compatibility
	if cfg.Port == "" {
		cfg.Port = getEnvWithFallback("BRIGHT_PORT", "HTTP_PORT", "PORT")
		if cfg.Port == "" {
			cfg.Port = "3000" // default
		}
	}

	if cfg.MasterKey == "" {
		cfg.MasterKey = getEnvWithFallback("BRIGHT_MASTER_KEY", "MASTER_KEY")
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = getEnvWithFallback("BRIGHT_LOG_LEVEL", "LOG_LEVEL")
		if cfg.LogLevel == "" {
			cfg.LogLevel = "info" // default
		}
	}

	if cfg.DataPath == "" {
		cfg.DataPath = getEnvWithFallback("BRIGHT_DATA_PATH", "DATA_PATH")
		if cfg.DataPath == "" {
			cfg.DataPath = "./data" // default
		}
	}

	// AutoCreateIndex needs special handling since it's a bool
	if autoCreateStr := getEnvWithFallback("BRIGHT_AUTO_CREATE_INDEX", "AUTO_CREATE_INDEX"); autoCreateStr != "" {
		cfg.AutoCreateIndex = autoCreateStr == "true" || autoCreateStr == "1"
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

// getEnvWithFallback returns the first non-empty environment variable from the list
func getEnvWithFallback(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
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

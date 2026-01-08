package config

import (
	"strings"

	"github.com/caarlos0/env/v11"
)

// Config holds the application configuration
type Config struct {
	Port      string `env:"PORT" envDefault:"3000"`
	MasterKey string `env:"BRIGHT_MASTER_KEY"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	DataPath  string `env:"DATA_PATH" envDefault:"./data"`

	// Raft configuration
	RaftEnabled   bool   `env:"RAFT_ENABLED" envDefault:"false"`
	RaftNodeID    string `env:"RAFT_NODE_ID"`
	RaftDir       string `env:"RAFT_DIR" envDefault:"./data/raft"`
	RaftBind      string `env:"RAFT_BIND" envDefault:"0.0.0.0:7000"`
	RaftBootstrap bool   `env:"RAFT_BOOTSTRAP" envDefault:"false"`
	RaftPeers     string `env:"RAFT_PEERS"` // Comma-separated

	// Kubernetes discovery
	K8sEnabled     bool   `env:"K8S_ENABLED" envDefault:"false"`
	K8sServiceName string `env:"K8S_SERVICE_NAME" envDefault:"bright"`
	K8sNamespace   string `env:"K8S_NAMESPACE" envDefault:"default"`
	RaftPort       int    `env:"RAFT_PORT" envDefault:"7000"`
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, err
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

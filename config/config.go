package config

import (
	"github.com/caarlos0/env/v11"
)

// Config holds the application configuration
type Config struct {
	Port      string `env:"PORT" envDefault:"3000"`
	MasterKey string `env:"BRIGHT_MASTER_KEY"`
	LogLevel  string `env:"LOG_LEVEL" envDefault:"info"`
	DataPath  string `env:"DATA_PATH" envDefault:"./data"`
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

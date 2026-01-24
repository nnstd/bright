package postgres

import (
	"fmt"
	"time"

	"github.com/bytedance/sonic"
)

// SyncMode defines how the ingress synchronizes data
type SyncMode string

const (
	SyncModePolling SyncMode = "polling"
	SyncModeListen  SyncMode = "listen"
)

// Config holds the configuration for a PostgreSQL ingress
type Config struct {
	// Connection settings
	DSN string `json:"dsn"` // PostgreSQL connection string

	// Table settings
	Schema  string   `json:"schema"`            // Schema name (default: "public")
	Table   string   `json:"table"`             // Table name to sync
	Columns []string `json:"columns,omitempty"` // Columns to sync (empty = all)

	// Primary key settings
	PrimaryKey string `json:"primary_key"` // Primary key column name

	// Column mapping: source column -> document field
	ColumnMapping map[string]string `json:"column_mapping,omitempty"`

	// Sync settings
	UpdatedAtColumn string   `json:"updated_at_column,omitempty"` // Column for incremental sync
	WhereClause     string   `json:"where_clause,omitempty"`      // Additional WHERE filter
	SyncMode        SyncMode `json:"sync_mode"`                   // polling or listen
	PollInterval    Duration `json:"poll_interval,omitempty"`     // Polling interval (default: 30s)
	BatchSize       int      `json:"batch_size,omitempty"`        // Documents per batch (default: 1000)

	// Trigger settings
	AutoTriggers  bool   `json:"auto_triggers"`            // Auto-create triggers
	NotifyChannel string `json:"notify_channel,omitempty"` // LISTEN/NOTIFY channel name
}

// Duration is a time.Duration that can be unmarshaled from JSON
type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := sonic.Unmarshal(b, &v); err != nil {
		return err
	}
	switch value := v.(type) {
	case float64:
		*d = Duration(time.Duration(value))
		return nil
	case string:
		dur, err := time.ParseDuration(value)
		if err != nil {
			return err
		}
		*d = Duration(dur)
		return nil
	default:
		return fmt.Errorf("invalid duration type: %T", v)
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.DSN == "" {
		return fmt.Errorf("dsn is required")
	}
	if c.Table == "" {
		return fmt.Errorf("table is required")
	}
	if c.PrimaryKey == "" {
		return fmt.Errorf("primary_key is required")
	}
	if c.SyncMode == "" {
		c.SyncMode = SyncModePolling
	}
	if c.SyncMode != SyncModePolling && c.SyncMode != SyncModeListen {
		return fmt.Errorf("sync_mode must be 'polling' or 'listen'")
	}
	if c.SyncMode == SyncModePolling && c.UpdatedAtColumn == "" {
		return fmt.Errorf("updated_at_column is required for polling mode")
	}
	return nil
}

// WithDefaults returns the config with default values applied
func (c *Config) WithDefaults() *Config {
	cfg := *c
	if cfg.Schema == "" {
		cfg.Schema = "public"
	}
	if cfg.SyncMode == "" {
		cfg.SyncMode = SyncModePolling
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = Duration(30 * time.Second)
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 1000
	}
	if cfg.NotifyChannel == "" {
		cfg.NotifyChannel = fmt.Sprintf("bright_%s", cfg.Table)
	}
	return &cfg
}

// FullTableName returns schema.table
func (c *Config) FullTableName() string {
	return fmt.Sprintf("%s.%s", c.Schema, c.Table)
}

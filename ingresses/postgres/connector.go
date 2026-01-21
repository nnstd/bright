package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Connector manages the PostgreSQL connection pool with automatic reconnection
type Connector struct {
	dsn    string
	pool   *pgxpool.Pool
	logger *zap.Logger

	maxConns    int32
	connTimeout time.Duration
}

// ConnectorConfig holds connection pool settings
type ConnectorConfig struct {
	DSN         string
	MaxConns    int32
	ConnTimeout time.Duration
}

// NewConnector creates a new Connector
func NewConnector(cfg ConnectorConfig, logger *zap.Logger) *Connector {
	if cfg.MaxConns == 0 {
		cfg.MaxConns = 10
	}
	if cfg.ConnTimeout == 0 {
		cfg.ConnTimeout = 30 * time.Second
	}

	return &Connector{
		dsn:         cfg.DSN,
		logger:      logger,
		maxConns:    cfg.MaxConns,
		connTimeout: cfg.ConnTimeout,
	}
}

// Connect establishes a connection to PostgreSQL
func (c *Connector) Connect(ctx context.Context) error {
	config, err := pgxpool.ParseConfig(c.dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}

	config.MaxConns = c.maxConns
	config.ConnConfig.ConnectTimeout = c.connTimeout

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.pool = pool
	c.logger.Info("Connected to PostgreSQL",
		zap.String("dsn", sanitizeDSN(c.dsn)),
		zap.Int32("max_conns", c.maxConns))

	return nil
}

// Pool returns the connection pool
func (c *Connector) Pool() *pgxpool.Pool {
	return c.pool
}

// Close closes the connection pool
func (c *Connector) Close() {
	if c.pool != nil {
		c.pool.Close()
		c.pool = nil
	}
}

// Reconnect attempts to reconnect with exponential backoff
func (c *Connector) Reconnect(ctx context.Context) error {
	c.Close()

	backoff := time.Second
	maxBackoff := 5 * time.Minute
	maxAttempts := 30

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		c.logger.Info("Attempting to reconnect to PostgreSQL",
			zap.Int("attempt", attempt),
			zap.Duration("backoff", backoff))

		err := c.Connect(ctx)
		if err == nil {
			c.logger.Info("Reconnected to PostgreSQL")
			return nil
		}

		c.logger.Warn("Failed to reconnect",
			zap.Error(err),
			zap.Duration("next_retry", backoff))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return fmt.Errorf("failed to reconnect after %d attempts", maxAttempts)
}

// IsConnected returns true if the pool is connected and healthy
func (c *Connector) IsConnected(ctx context.Context) bool {
	if c.pool == nil {
		return false
	}
	return c.pool.Ping(ctx) == nil
}

// sanitizeDSN removes password from DSN for logging
func sanitizeDSN(_ string) string {
	// Simple sanitization - in production you'd want more robust parsing
	// This just indicates we have a DSN without exposing credentials
	return "[redacted]"
}

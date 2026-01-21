package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Schema handles DDL operations for sync tables and triggers
type Schema struct {
	pool   *pgxpool.Pool
	config *Config
}

// NewSchema creates a new Schema handler
func NewSchema(pool *pgxpool.Pool, config *Config) *Schema {
	return &Schema{pool: pool, config: config}
}

// CreateSyncTables creates the __bright_synchronization tables if they don't exist
func (s *Schema) CreateSyncTables(ctx context.Context) error {
	// Create sync state table
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS __bright_synchronization (
			table_name VARCHAR(255) PRIMARY KEY,
			last_sync_at TIMESTAMPTZ,
			last_id TEXT,
			full_sync_complete BOOLEAN DEFAULT FALSE,
			updated_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create __bright_synchronization table: %w", err)
	}

	// Create delete tracking table
	_, err = s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS __bright_synchronization_deletes (
			id SERIAL PRIMARY KEY,
			source_table VARCHAR(255) NOT NULL,
			deleted_id TEXT NOT NULL,
			deleted_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create __bright_synchronization_deletes table: %w", err)
	}

	// Create index on delete tracking table
	_, err = s.pool.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_bright_deletes_table_time
		ON __bright_synchronization_deletes(source_table, deleted_at)
	`)
	if err != nil {
		return fmt.Errorf("failed to create index on __bright_synchronization_deletes: %w", err)
	}

	return nil
}

// CreateDeleteTrigger creates the trigger for tracking hard deletes
func (s *Schema) CreateDeleteTrigger(ctx context.Context) error {
	tableName := s.config.Table
	fullTable := s.config.FullTableName()
	primaryKey := s.config.PrimaryKey

	// Create trigger function
	funcName := fmt.Sprintf("__bright_track_deletes_%s", tableName)
	_, err := s.pool.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s()
		RETURNS TRIGGER AS $$
		BEGIN
			INSERT INTO __bright_synchronization_deletes (source_table, deleted_id)
			VALUES ('%s', OLD.%s::TEXT);
			RETURN OLD;
		END;
		$$ LANGUAGE plpgsql
	`, funcName, tableName, primaryKey))
	if err != nil {
		return fmt.Errorf("failed to create delete tracking function: %w", err)
	}

	// Create trigger
	triggerName := fmt.Sprintf("__bright_delete_trigger_%s", tableName)
	_, err = s.pool.Exec(ctx, fmt.Sprintf(`
		DROP TRIGGER IF EXISTS %s ON %s;
		CREATE TRIGGER %s
		AFTER DELETE ON %s
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, triggerName, fullTable, triggerName, fullTable, funcName))
	if err != nil {
		return fmt.Errorf("failed to create delete trigger: %w", err)
	}

	return nil
}

// CreateNotifyTrigger creates the trigger for LISTEN/NOTIFY mode
func (s *Schema) CreateNotifyTrigger(ctx context.Context) error {
	tableName := s.config.Table
	fullTable := s.config.FullTableName()
	primaryKey := s.config.PrimaryKey
	channel := s.config.NotifyChannel

	// Create trigger function
	funcName := fmt.Sprintf("__bright_notify_%s", tableName)
	_, err := s.pool.Exec(ctx, fmt.Sprintf(`
		CREATE OR REPLACE FUNCTION %s()
		RETURNS TRIGGER AS $$
		BEGIN
			PERFORM pg_notify('%s',
				json_build_object(
					'op', TG_OP,
					'id', COALESCE(NEW.%s, OLD.%s)::TEXT
				)::TEXT
			);
			RETURN COALESCE(NEW, OLD);
		END;
		$$ LANGUAGE plpgsql
	`, funcName, channel, primaryKey, primaryKey))
	if err != nil {
		return fmt.Errorf("failed to create notify function: %w", err)
	}

	// Create trigger
	triggerName := fmt.Sprintf("__bright_notify_trigger_%s", tableName)
	_, err = s.pool.Exec(ctx, fmt.Sprintf(`
		DROP TRIGGER IF EXISTS %s ON %s;
		CREATE TRIGGER %s
		AFTER INSERT OR UPDATE OR DELETE ON %s
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, triggerName, fullTable, triggerName, fullTable, funcName))
	if err != nil {
		return fmt.Errorf("failed to create notify trigger: %w", err)
	}

	return nil
}

// DropTriggers removes all bright triggers from the table
func (s *Schema) DropTriggers(ctx context.Context) error {
	tableName := s.config.Table
	fullTable := s.config.FullTableName()

	// Drop delete trigger
	deleteTrigger := fmt.Sprintf("__bright_delete_trigger_%s", tableName)
	_, _ = s.pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, deleteTrigger, fullTable))

	// Drop notify trigger
	notifyTrigger := fmt.Sprintf("__bright_notify_trigger_%s", tableName)
	_, _ = s.pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, notifyTrigger, fullTable))

	// Drop functions
	deleteFunc := fmt.Sprintf("__bright_track_deletes_%s", tableName)
	_, _ = s.pool.Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, deleteFunc))

	notifyFunc := fmt.Sprintf("__bright_notify_%s", tableName)
	_, _ = s.pool.Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, notifyFunc))

	return nil
}

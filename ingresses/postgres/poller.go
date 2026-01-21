package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Poller handles polling-based synchronization from PostgreSQL
type Poller struct {
	pool   *pgxpool.Pool
	config *Config
	mapper *Mapper
	logger *zap.Logger

	// Callbacks
	onDocuments func(docs []map[string]any) error
	onDeletes   func(ids []string) error

	// State
	lastSyncAt       time.Time
	lastID           string
	fullSyncComplete bool
}

// NewPoller creates a new Poller
func NewPoller(pool *pgxpool.Pool, config *Config, logger *zap.Logger) *Poller {
	return &Poller{
		pool:   pool,
		config: config,
		mapper: NewMapper(config),
		logger: logger,
	}
}

// SetCallbacks sets the callbacks for document and delete processing
func (p *Poller) SetCallbacks(onDocuments func(docs []map[string]any) error, onDeletes func(ids []string) error) {
	p.onDocuments = onDocuments
	p.onDeletes = onDeletes
}

// SetState sets the initial sync state
func (p *Poller) SetState(lastSyncAt time.Time, lastID string, fullSyncComplete bool) {
	p.lastSyncAt = lastSyncAt
	p.lastID = lastID
	p.fullSyncComplete = fullSyncComplete
}

// GetState returns the current sync state
func (p *Poller) GetState() (time.Time, string, bool) {
	return p.lastSyncAt, p.lastID, p.fullSyncComplete
}

// Poll performs a single poll cycle
func (p *Poller) Poll(ctx context.Context) error {
	if !p.fullSyncComplete {
		return p.fullSync(ctx)
	}
	return p.incrementalSync(ctx)
}

// fullSync performs a full table synchronization
func (p *Poller) fullSync(ctx context.Context) error {
	p.logger.Info("Starting full sync",
		zap.String("table", p.config.FullTableName()))

	totalDocs := 0
	for {
		docs, lastID, err := p.fetchBatch(ctx, p.lastID)
		if err != nil {
			return fmt.Errorf("failed to fetch batch: %w", err)
		}

		if len(docs) == 0 {
			break
		}

		if p.onDocuments != nil {
			if err := p.onDocuments(docs); err != nil {
				return fmt.Errorf("failed to process documents: %w", err)
			}
		}

		p.lastID = lastID
		totalDocs += len(docs)

		p.logger.Debug("Full sync batch processed",
			zap.Int("batch_size", len(docs)),
			zap.Int("total", totalDocs),
			zap.String("last_id", lastID))

		if len(docs) < p.config.BatchSize {
			break
		}
	}

	p.fullSyncComplete = true
	p.lastSyncAt = time.Now()
	p.lastID = ""

	p.logger.Info("Full sync completed",
		zap.String("table", p.config.FullTableName()),
		zap.Int("documents", totalDocs))

	return nil
}

// incrementalSync fetches and processes changes since last sync
func (p *Poller) incrementalSync(ctx context.Context) error {
	// Sync updates/inserts
	docs, err := p.fetchChanges(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch changes: %w", err)
	}

	if len(docs) > 0 && p.onDocuments != nil {
		if err := p.onDocuments(docs); err != nil {
			return fmt.Errorf("failed to process documents: %w", err)
		}
		p.logger.Debug("Incremental sync: processed updates",
			zap.Int("count", len(docs)))
	}

	// Sync deletes
	deleteIDs, err := p.fetchDeletes(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch deletes: %w", err)
	}

	if len(deleteIDs) > 0 && p.onDeletes != nil {
		if err := p.onDeletes(deleteIDs); err != nil {
			return fmt.Errorf("failed to process deletes: %w", err)
		}
		p.logger.Debug("Incremental sync: processed deletes",
			zap.Int("count", len(deleteIDs)))
	}

	p.lastSyncAt = time.Now()
	return nil
}

// fetchBatch fetches a batch of documents for full sync
func (p *Poller) fetchBatch(ctx context.Context, afterID string) ([]map[string]any, string, error) {
	columns := "*"
	if len(p.config.Columns) > 0 {
		columns = strings.Join(p.config.Columns, ", ")
	}

	var query string
	var args []any

	if afterID == "" {
		query = fmt.Sprintf(`
			SELECT %s FROM %s
			%s
			ORDER BY %s
			LIMIT $1
		`, columns, p.config.FullTableName(), p.whereClause(), p.config.PrimaryKey)
		args = []any{p.config.BatchSize}
	} else {
		query = fmt.Sprintf(`
			SELECT %s FROM %s
			WHERE %s > $1 %s
			ORDER BY %s
			LIMIT $2
		`, columns, p.config.FullTableName(), p.config.PrimaryKey, p.andWhereClause(), p.config.PrimaryKey)
		args = []any{afterID, p.config.BatchSize}
	}

	rows, err := p.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var docs []map[string]any
	var lastID string

	for rows.Next() {
		doc, err := p.mapper.RowToDocument(rows)
		if err != nil {
			p.logger.Warn("Failed to map row", zap.Error(err))
			continue
		}

		id, err := p.mapper.GetPrimaryKeyValue(doc)
		if err != nil {
			p.logger.Warn("Failed to get primary key", zap.Error(err))
			continue
		}

		lastID = id
		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("rows iteration error: %w", err)
	}

	return docs, lastID, nil
}

// fetchChanges fetches documents changed since last sync
func (p *Poller) fetchChanges(ctx context.Context) ([]map[string]any, error) {
	columns := "*"
	if len(p.config.Columns) > 0 {
		columns = strings.Join(p.config.Columns, ", ")
	}

	query := fmt.Sprintf(`
		SELECT %s FROM %s
		WHERE %s > $1 %s
		ORDER BY %s
		LIMIT $2
	`, columns, p.config.FullTableName(), p.config.UpdatedAtColumn, p.andWhereClause(), p.config.UpdatedAtColumn)

	rows, err := p.pool.Query(ctx, query, p.lastSyncAt, p.config.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var docs []map[string]any
	for rows.Next() {
		doc, err := p.mapper.RowToDocument(rows)
		if err != nil {
			p.logger.Warn("Failed to map row", zap.Error(err))
			continue
		}
		docs = append(docs, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return docs, nil
}

// fetchDeletes fetches deleted IDs from the tracking table
func (p *Poller) fetchDeletes(ctx context.Context) ([]string, error) {
	query := `
		SELECT deleted_id FROM __bright_synchronization_deletes
		WHERE source_table = $1 AND deleted_at > $2
		ORDER BY deleted_at
		LIMIT $3
	`

	rows, err := p.pool.Query(ctx, query, p.config.Table, p.lastSyncAt, p.config.BatchSize)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			p.logger.Warn("Failed to scan delete ID", zap.Error(err))
			continue
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return ids, nil
}

// whereClause returns the WHERE clause for queries
func (p *Poller) whereClause() string {
	if p.config.WhereClause == "" {
		return ""
	}
	return "WHERE " + p.config.WhereClause
}

// andWhereClause returns an AND clause for additional conditions
func (p *Poller) andWhereClause() string {
	if p.config.WhereClause == "" {
		return ""
	}
	return "AND " + p.config.WhereClause
}

// ResetState resets the sync state for a full resync
func (p *Poller) ResetState() {
	p.lastSyncAt = time.Time{}
	p.lastID = ""
	p.fullSyncComplete = false
}

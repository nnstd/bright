package postgres

import (
	"bright/ingresses"
	"bright/raft"
	"bright/store"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

// Ingress implements the ingresses.Ingress interface for PostgreSQL
type Ingress struct {
	id       string
	indexID  string
	config   *Config
	rawConfig json.RawMessage

	connector *Connector
	schema    *Schema
	poller    *Poller
	listener  *Listener
	mapper    *Mapper

	store    *store.IndexStore
	raftNode *raft.RaftNode
	logger   *zap.Logger

	status atomic.Value // ingresses.Status
	stats  struct {
		sync.RWMutex
		lastSyncAt       time.Time
		documentsSynced  int64
		documentsDeleted int64
		fullSyncComplete bool
		lastError        string
		errorCount       int
	}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// NewIngress creates a new PostgreSQL ingress
func NewIngress(cfg ingresses.Config, store *store.IndexStore, raftNode *raft.RaftNode, logger *zap.Logger) (*Ingress, error) {
	// Parse the postgres-specific config
	var pgConfig Config
	if err := sonic.Unmarshal(cfg.Config, &pgConfig); err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	// Validate and apply defaults
	if err := pgConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid postgres config: %w", err)
	}
	pgConfigWithDefaults := pgConfig.WithDefaults()

	ing := &Ingress{
		id:        cfg.ID,
		indexID:   cfg.IndexID,
		config:    pgConfigWithDefaults,
		rawConfig: cfg.Config,
		store:     store,
		raftNode:  raftNode,
		logger:    logger.With(zap.String("ingress_id", cfg.ID), zap.String("index_id", cfg.IndexID)),
		mapper:    NewMapper(pgConfigWithDefaults),
	}

	ing.status.Store(ingresses.StatusStopped)

	return ing, nil
}

// Factory returns a factory function for creating PostgreSQL ingresses
func Factory(cfg ingresses.Config, store *store.IndexStore, raftNode *raft.RaftNode, logger *zap.Logger) (ingresses.Ingress, error) {
	return NewIngress(cfg, store, raftNode, logger)
}

// ID returns the ingress ID
func (i *Ingress) ID() string {
	return i.id
}

// IndexID returns the target index ID
func (i *Ingress) IndexID() string {
	return i.indexID
}

// Type returns the ingress type
func (i *Ingress) Type() string {
	return "postgres"
}

// Status returns the current status
func (i *Ingress) Status() ingresses.Status {
	return i.status.Load().(ingresses.Status)
}

// Config returns the raw configuration
func (i *Ingress) Config() json.RawMessage {
	return i.rawConfig
}

// Stats returns the current statistics
func (i *Ingress) Stats() ingresses.Stats {
	i.stats.RLock()
	defer i.stats.RUnlock()

	return ingresses.Stats{
		LastSyncAt:       i.stats.lastSyncAt,
		DocumentsSynced:  i.stats.documentsSynced,
		DocumentsDeleted: i.stats.documentsDeleted,
		FullSyncComplete: i.stats.fullSyncComplete,
		LastError:        i.stats.lastError,
		ErrorCount:       i.stats.errorCount,
	}
}

// Start begins synchronization
func (i *Ingress) Start(ctx context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.Status() == ingresses.StatusRunning {
		return nil // Already running
	}

	i.status.Store(ingresses.StatusStarting)
	i.logger.Info("Starting PostgreSQL ingress")

	// Create context for this ingress
	if ctx == nil {
		ctx = context.Background()
	}
	i.ctx, i.cancel = context.WithCancel(ctx)

	// Create connector
	i.connector = NewConnector(ConnectorConfig{
		DSN:         i.config.DSN,
		MaxConns:    10,
		ConnTimeout: 30 * time.Second,
	}, i.logger)

	// Connect to PostgreSQL
	if err := i.connector.Connect(i.ctx); err != nil {
		i.setError(fmt.Sprintf("connection failed: %v", err))
		return err
	}

	// Create schema handler and ensure tables exist
	i.schema = NewSchema(i.connector.Pool(), i.config)
	if err := i.schema.CreateSyncTables(i.ctx); err != nil {
		i.setError(fmt.Sprintf("failed to create sync tables: %v", err))
		return err
	}

	// Create triggers if auto_triggers is enabled
	if i.config.AutoTriggers {
		if err := i.schema.CreateDeleteTrigger(i.ctx); err != nil {
			i.logger.Warn("Failed to create delete trigger", zap.Error(err))
		}
		if i.config.SyncMode == SyncModeListen {
			if err := i.schema.CreateNotifyTrigger(i.ctx); err != nil {
				i.logger.Warn("Failed to create notify trigger", zap.Error(err))
			}
		}
	}

	// Load sync state
	i.loadState()

	// Start sync based on mode
	if i.config.SyncMode == SyncModeListen {
		if err := i.startListenMode(); err != nil {
			i.setError(fmt.Sprintf("failed to start listen mode: %v", err))
			return err
		}
	} else {
		i.startPollingMode()
	}

	i.status.Store(ingresses.StatusRunning)
	i.logger.Info("PostgreSQL ingress started",
		zap.String("sync_mode", string(i.config.SyncMode)))

	return nil
}

// Stop halts synchronization
func (i *Ingress) Stop() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.Status() == ingresses.StatusStopped {
		return nil
	}

	i.logger.Info("Stopping PostgreSQL ingress")

	if i.cancel != nil {
		i.cancel()
	}

	i.wg.Wait()

	if i.listener != nil {
		i.listener.Stop()
	}

	if i.connector != nil {
		i.connector.Close()
	}

	// Save state before stopping
	i.saveState()

	i.status.Store(ingresses.StatusStopped)
	i.logger.Info("PostgreSQL ingress stopped")

	return nil
}

// Pause temporarily pauses synchronization
func (i *Ingress) Pause() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.Status() != ingresses.StatusRunning {
		return fmt.Errorf("ingress is not running")
	}

	i.status.Store(ingresses.StatusPaused)
	i.logger.Info("PostgreSQL ingress paused")
	return nil
}

// Resume resumes a paused synchronization
func (i *Ingress) Resume() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.Status() != ingresses.StatusPaused {
		return fmt.Errorf("ingress is not paused")
	}

	i.status.Store(ingresses.StatusRunning)
	i.logger.Info("PostgreSQL ingress resumed")
	return nil
}

// Resync triggers a full resynchronization
func (i *Ingress) Resync() error {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.logger.Info("Triggering full resync")

	// Reset state
	i.stats.Lock()
	i.stats.fullSyncComplete = false
	i.stats.documentsSynced = 0
	i.stats.documentsDeleted = 0
	i.stats.Unlock()

	if i.poller != nil {
		i.poller.ResetState()
	}

	// Clear sync state in database
	if i.connector != nil && i.connector.Pool() != nil {
		_, err := i.connector.Pool().Exec(i.ctx,
			"DELETE FROM __bright_synchronization WHERE table_name = $1",
			i.config.Table)
		if err != nil {
			i.logger.Warn("Failed to clear sync state", zap.Error(err))
		}
	}

	return nil
}

// startPollingMode starts the polling sync loop
func (i *Ingress) startPollingMode() {
	i.poller = NewPoller(i.connector.Pool(), i.config, i.logger)
	i.poller.SetCallbacks(i.handleDocuments, i.handleDeletes)

	// Set initial state
	i.stats.RLock()
	i.poller.SetState(i.stats.lastSyncAt, "", i.stats.fullSyncComplete)
	i.stats.RUnlock()

	i.wg.Add(1)
	go func() {
		defer i.wg.Done()
		i.pollLoop()
	}()
}

// pollLoop runs the polling loop
func (i *Ingress) pollLoop() {
	ticker := time.NewTicker(i.config.PollInterval.Duration())
	defer ticker.Stop()

	// Initial poll
	i.doPoll()

	for {
		select {
		case <-i.ctx.Done():
			return
		case <-ticker.C:
			if i.Status() == ingresses.StatusPaused {
				continue
			}
			i.doPoll()
		}
	}
}

// doPoll performs a single poll cycle
func (i *Ingress) doPoll() {
	i.status.Store(ingresses.StatusSyncing)
	defer func() {
		if i.Status() == ingresses.StatusSyncing {
			i.status.Store(ingresses.StatusRunning)
		}
	}()

	if err := i.poller.Poll(i.ctx); err != nil {
		i.setError(fmt.Sprintf("poll failed: %v", err))
		return
	}

	// Update state from poller
	lastSyncAt, _, fullSyncComplete := i.poller.GetState()
	i.stats.Lock()
	i.stats.lastSyncAt = lastSyncAt
	i.stats.fullSyncComplete = fullSyncComplete
	i.stats.Unlock()

	// Persist state
	i.saveState()
}

// startListenMode starts the LISTEN/NOTIFY sync
func (i *Ingress) startListenMode() error {
	// First, do a full sync using poller
	i.poller = NewPoller(i.connector.Pool(), i.config, i.logger)
	i.poller.SetCallbacks(i.handleDocuments, i.handleDeletes)

	i.stats.RLock()
	fullSyncComplete := i.stats.fullSyncComplete
	i.stats.RUnlock()

	if !fullSyncComplete {
		i.logger.Info("Performing initial full sync before listening")
		if err := i.poller.Poll(i.ctx); err != nil {
			return fmt.Errorf("initial sync failed: %w", err)
		}

		lastSyncAt, _, complete := i.poller.GetState()
		i.stats.Lock()
		i.stats.lastSyncAt = lastSyncAt
		i.stats.fullSyncComplete = complete
		i.stats.Unlock()
		i.saveState()
	}

	// Start listener
	i.listener = NewListener(i.connector.Pool(), i.config, i.logger)
	i.listener.SetCallback(i.handleNotify)

	return i.listener.Start(i.ctx)
}

// handleDocuments processes synced documents
func (i *Ingress) handleDocuments(docs []map[string]any) error {
	if len(docs) == 0 {
		return nil
	}

	// Use Raft if enabled, otherwise direct store access
	if i.raftNode != nil && i.raftNode.IsLeader() {
		return i.applyDocumentsViaRaft(docs)
	}

	err := i.store.AddDocumentsInternal(i.indexID, docs)
	if err != nil {
		return err
	}

	i.stats.Lock()
	i.stats.documentsSynced += int64(len(docs))
	i.stats.Unlock()

	return nil
}

// handleDeletes processes deleted document IDs
func (i *Ingress) handleDeletes(ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	err := i.store.DeleteDocumentsInternal(i.indexID, "", ids)
	if err != nil {
		return err
	}

	i.stats.Lock()
	i.stats.documentsDeleted += int64(len(ids))
	i.stats.Unlock()

	return nil
}

// handleNotify processes a LISTEN/NOTIFY event
func (i *Ingress) handleNotify(op string, id string) error {
	switch op {
	case "INSERT", "UPDATE":
		// Fetch the document and sync it
		doc, err := i.fetchDocument(id)
		if err != nil {
			return err
		}
		if doc != nil {
			return i.handleDocuments([]map[string]any{doc})
		}
	case "DELETE":
		return i.handleDeletes([]string{id})
	}
	return nil
}

// fetchDocument fetches a single document by primary key
func (i *Ingress) fetchDocument(id string) (map[string]any, error) {
	columns := "*"
	if len(i.config.Columns) > 0 {
		columns = strings.Join(i.config.Columns, ", ")
	}

	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1",
		columns, i.config.FullTableName(), i.config.PrimaryKey)

	rows, err := i.connector.Pool().Query(i.ctx, query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		return i.mapper.RowToDocument(rows)
	}

	return nil, nil
}

// applyDocumentsViaRaft applies documents through Raft consensus
func (i *Ingress) applyDocumentsViaRaft(docs []map[string]any) error {
	payload := raft.AddDocumentsPayload{
		IndexID:   i.indexID,
		Documents: docs,
	}

	payloadData, err := sonic.Marshal(payload)
	if err != nil {
		return err
	}

	cmd := raft.Command{
		Type: raft.CommandAddDocuments,
		Data: payloadData,
	}

	if err := i.raftNode.Apply(cmd, 30*time.Second); err != nil {
		return err
	}

	i.stats.Lock()
	i.stats.documentsSynced += int64(len(docs))
	i.stats.Unlock()

	return nil
}

// loadState loads the sync state from PostgreSQL
func (i *Ingress) loadState() {
	if i.connector == nil || i.connector.Pool() == nil {
		return
	}

	var lastSyncAt *time.Time
	var lastID *string
	var fullSyncComplete bool

	err := i.connector.Pool().QueryRow(i.ctx,
		"SELECT last_sync_at, last_id, full_sync_complete FROM __bright_synchronization WHERE table_name = $1",
		i.config.Table).Scan(&lastSyncAt, &lastID, &fullSyncComplete)

	if err != nil {
		// No state found, start fresh
		return
	}

	i.stats.Lock()
	if lastSyncAt != nil {
		i.stats.lastSyncAt = *lastSyncAt
	}
	i.stats.fullSyncComplete = fullSyncComplete
	i.stats.Unlock()
}

// saveState persists the sync state to PostgreSQL
func (i *Ingress) saveState() {
	if i.connector == nil || i.connector.Pool() == nil {
		return
	}

	i.stats.RLock()
	lastSyncAt := i.stats.lastSyncAt
	fullSyncComplete := i.stats.fullSyncComplete
	i.stats.RUnlock()

	_, err := i.connector.Pool().Exec(i.ctx, `
		INSERT INTO __bright_synchronization (table_name, last_sync_at, full_sync_complete, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (table_name) DO UPDATE SET
			last_sync_at = EXCLUDED.last_sync_at,
			full_sync_complete = EXCLUDED.full_sync_complete,
			updated_at = NOW()
	`, i.config.Table, lastSyncAt, fullSyncComplete)

	if err != nil {
		i.logger.Warn("Failed to save sync state", zap.Error(err))
	}
}

// setError sets an error state
func (i *Ingress) setError(msg string) {
	i.stats.Lock()
	i.stats.lastError = msg
	i.stats.errorCount++
	i.stats.Unlock()
	i.status.Store(ingresses.StatusError)
	i.logger.Error("Ingress error", zap.String("error", msg))
}

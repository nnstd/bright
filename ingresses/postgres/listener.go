package postgres

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// NotifyPayload represents the JSON payload from pg_notify
type NotifyPayload struct {
	Op string `json:"op"` // INSERT, UPDATE, DELETE
	ID string `json:"id"` // Primary key value
}

// Listener handles LISTEN/NOTIFY based synchronization
type Listener struct {
	pool   *pgxpool.Pool
	config *Config
	logger *zap.Logger

	// Callbacks
	onNotify func(op string, id string) error

	// Batching
	batchMu      sync.Mutex
	pendingOps   []NotifyPayload
	batchTimeout time.Duration
	batchSize    int

	// Control
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewListener creates a new Listener
func NewListener(pool *pgxpool.Pool, config *Config, logger *zap.Logger) *Listener {
	return &Listener{
		pool:         pool,
		config:       config,
		logger:       logger,
		batchTimeout: 100 * time.Millisecond,
		batchSize:    100,
	}
}

// SetCallback sets the callback for notification processing
func (l *Listener) SetCallback(onNotify func(op string, id string) error) {
	l.onNotify = onNotify
}

// Start begins listening for notifications
func (l *Listener) Start(ctx context.Context) error {
	ctx, l.cancel = context.WithCancel(ctx)

	// Acquire a dedicated connection for LISTEN
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire connection: %w", err)
	}

	// Subscribe to the channel
	channel := l.config.NotifyChannel
	_, err = conn.Exec(ctx, fmt.Sprintf("LISTEN %s", channel))
	if err != nil {
		conn.Release()
		return fmt.Errorf("failed to LISTEN on channel %s: %w", channel, err)
	}

	l.logger.Info("Listening for notifications",
		zap.String("channel", channel))

	// Start the listener goroutine
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		defer conn.Release()
		l.listenLoop(ctx, conn)
	}()

	// Start the batch processor
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.batchProcessor(ctx)
	}()

	return nil
}

// Stop stops the listener
func (l *Listener) Stop() {
	if l.cancel != nil {
		l.cancel()
	}
	l.wg.Wait()
}

// listenLoop continuously waits for notifications
func (l *Listener) listenLoop(ctx context.Context, conn *pgxpool.Conn) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		notification, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // Context cancelled
			}
			l.logger.Error("Error waiting for notification", zap.Error(err))
			// Try to reconnect
			time.Sleep(time.Second)
			continue
		}

		var payload NotifyPayload
		if err := sonic.UnmarshalString(notification.Payload, &payload); err != nil {
			l.logger.Warn("Failed to parse notification payload",
				zap.String("payload", notification.Payload),
				zap.Error(err))
			continue
		}

		l.addToBatch(payload)
	}
}

// addToBatch adds a notification to the pending batch
func (l *Listener) addToBatch(payload NotifyPayload) {
	l.batchMu.Lock()
	defer l.batchMu.Unlock()

	l.pendingOps = append(l.pendingOps, payload)

	// If batch is full, process immediately
	if len(l.pendingOps) >= l.batchSize {
		l.processBatchLocked()
	}
}

// batchProcessor periodically processes pending batches
func (l *Listener) batchProcessor(ctx context.Context) {
	ticker := time.NewTicker(l.batchTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Process any remaining items
			l.batchMu.Lock()
			if len(l.pendingOps) > 0 {
				l.processBatchLocked()
			}
			l.batchMu.Unlock()
			return
		case <-ticker.C:
			l.batchMu.Lock()
			if len(l.pendingOps) > 0 {
				l.processBatchLocked()
			}
			l.batchMu.Unlock()
		}
	}
}

// processBatchLocked processes the pending batch (must be called with lock held)
func (l *Listener) processBatchLocked() {
	if l.onNotify == nil || len(l.pendingOps) == 0 {
		return
	}

	ops := l.pendingOps
	l.pendingOps = nil

	// Process outside the lock
	go func() {
		for _, op := range ops {
			if err := l.onNotify(op.Op, op.ID); err != nil {
				l.logger.Error("Failed to process notification",
					zap.String("op", op.Op),
					zap.String("id", op.ID),
					zap.Error(err))
			}
		}
	}()
}

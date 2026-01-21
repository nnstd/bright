package ingresses

import (
	"context"
	"encoding/json"
	"time"
)

// Status represents the current state of an ingress
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusPaused   Status = "paused"
	StatusFailed   Status = "failed"
	StatusSyncing  Status = "syncing"
)

// Statistics contains synchronization statistics
type Statistics struct {
	LastSyncAt       time.Time `json:"last_sync_at,omitempty"`
	DocumentsSynced  int64     `json:"documents_synced"`
	DocumentsDeleted int64     `json:"documents_deleted"`
	FullSyncComplete bool      `json:"full_sync_complete"`
	LastError        string    `json:"last_error,omitempty"`
	ErrorCount       int       `json:"error_count"`
}

// Ingress represents a data source that syncs to an index
type Ingress interface {
	// ID returns the unique identifier of this ingress
	ID() string

	// IndexID returns the target index ID
	IndexID() string

	// Type returns the ingress type (e.g., "postgres")
	Type() string

	// Status returns the current status
	Status() Status

	// Start begins synchronization
	Start(ctx context.Context) error

	// Stop halts synchronization
	Stop() error

	// Pause temporarily pauses synchronization
	Pause() error

	// Resume resumes a paused synchronization
	Resume() error

	// Resync triggers a full resynchronization
	Resync() error

	// Statistics returns current synchronization statistics
	Statistics() Statistics

	// Config returns the ingress configuration
	Config() json.RawMessage
}

// Config is the base configuration for all ingress types
type Config struct {
	ID      string          `json:"id"`
	IndexID string          `json:"index_id"`
	Type    string          `json:"type"`
	Config  json.RawMessage `json:"config"`
}

// IngressInfo contains information about an ingress for API responses
type IngressInfo struct {
	ID      string          `json:"id"`
	IndexID string          `json:"index_id"`
	Type    string          `json:"type"`
	Status  Status          `json:"status"`
	Config  json.RawMessage `json:"config"`
	Statistics   Statistics           `json:"stats"`
}

// ToInfo converts an Ingress to IngressInfo for API responses
func ToInfo(i Ingress) IngressInfo {
	return IngressInfo{
		ID:      i.ID(),
		IndexID: i.IndexID(),
		Type:    i.Type(),
		Status:  i.Status(),
		Config:  i.Config(),
		Statistics:   i.Statistics(),
	}
}

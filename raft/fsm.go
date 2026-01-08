package raft

import (
	"bright/models"
	"bright/store"
	"encoding/json"
	"fmt"
	"io"

	"github.com/hashicorp/raft"
)

// FSM implements the Raft finite state machine interface
// All state mutations flow through Apply() to ensure consistency
type FSM struct {
	store *store.IndexStore
}

// NewFSM creates a new FSM with the given store
func NewFSM(store *store.IndexStore) *FSM {
	return &FSM{store: store}
}

// Apply applies a Raft log entry to the FSM
// This is called by Raft when a command has been committed
func (f *FSM) Apply(log *raft.Log) interface{} {
	var cmd Command
	if err := json.Unmarshal(log.Data, &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command: %w", err)
	}

	switch cmd.Type {
	case CommandCreateIndex:
		return f.applyCreateIndex(cmd.Data)
	case CommandDeleteIndex:
		return f.applyDeleteIndex(cmd.Data)
	case CommandUpdateIndex:
		return f.applyUpdateIndex(cmd.Data)
	case CommandAddDocuments:
		return f.applyAddDocuments(cmd.Data)
	case CommandDeleteDocument:
		return f.applyDeleteDocument(cmd.Data)
	case CommandDeleteDocuments:
		return f.applyDeleteDocuments(cmd.Data)
	case CommandUpdateDocument:
		return f.applyUpdateDocument(cmd.Data)
	default:
		return fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// Snapshot returns a snapshot of the current FSM state
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	return &fsmSnapshot{store: f.store}, nil
}

// Restore restores the FSM from a snapshot
func (f *FSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()

	// Read snapshot data
	var configs map[string]*models.IndexConfig
	if err := json.NewDecoder(rc).Decode(&configs); err != nil {
		return fmt.Errorf("failed to decode snapshot: %w", err)
	}

	// Restore configuration metadata
	return f.store.RestoreConfigs(configs)
}

// Index operation apply methods

func (f *FSM) applyCreateIndex(data json.RawMessage) interface{} {
	var payload CreateIndexPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	config := &models.IndexConfig{
		ID:         payload.ID,
		PrimaryKey: payload.PrimaryKey,
	}

	return f.store.CreateIndexInternal(config)
}

func (f *FSM) applyDeleteIndex(data json.RawMessage) interface{} {
	var payload DeleteIndexPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	return f.store.DeleteIndexInternal(payload.ID)
}

func (f *FSM) applyUpdateIndex(data json.RawMessage) interface{} {
	var payload UpdateIndexPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	config := &models.IndexConfig{
		ID:         payload.ID,
		PrimaryKey: payload.PrimaryKey,
	}

	return f.store.UpdateIndexInternal(payload.ID, config)
}

// Document operation apply methods

func (f *FSM) applyAddDocuments(data json.RawMessage) interface{} {
	var payload AddDocumentsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	return f.store.AddDocumentsInternal(payload.IndexID, payload.Documents)
}

func (f *FSM) applyDeleteDocument(data json.RawMessage) interface{} {
	var payload DeleteDocumentPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	return f.store.DeleteDocumentInternal(payload.IndexID, payload.DocumentID)
}

func (f *FSM) applyDeleteDocuments(data json.RawMessage) interface{} {
	var payload DeleteDocumentsPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	return f.store.DeleteDocumentsInternal(payload.IndexID, payload.Filter, payload.IDs)
}

func (f *FSM) applyUpdateDocument(data json.RawMessage) interface{} {
	var payload UpdateDocumentPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	return f.store.UpdateDocumentInternal(payload.IndexID, payload.DocumentID, payload.Updates)
}

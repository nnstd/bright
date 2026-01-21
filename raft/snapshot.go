package raft

import (
	"bright/store"

	"github.com/bytedance/sonic"
	"github.com/hashicorp/raft"
)

// fsmSnapshot represents a point-in-time snapshot of the FSM state
type fsmSnapshot struct {
	store *store.IndexStore
}

// Persist saves the FSM snapshot to the provided sink
// Only index configurations are saved (not Bleve index data)
func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	// Get all index configurations
	configs := s.store.GetAllConfigs()

	// Serialize configurations to JSON
	data, err := sonic.Marshal(configs)
	if err != nil {
		sink.Cancel()
		return err
	}

	// Write to sink
	if _, err := sink.Write(data); err != nil {
		sink.Cancel()
		return err
	}

	return sink.Close()
}

// Release is called when we are finished with the snapshot
func (s *fsmSnapshot) Release() {
	// No-op: IndexStore is shared, not cloned
}

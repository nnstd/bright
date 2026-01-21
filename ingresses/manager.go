package ingresses

import (
	"bright/raft"
	"bright/store"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/bytedance/sonic"
	"go.uber.org/zap"
)

// Factory is a function that creates an Ingress from configuration
type Factory func(cfg Config, store *store.IndexStore, raftNode *raft.RaftNode, logger *zap.Logger) (Ingress, error)

// Manager manages all ingresses and their lifecycle
type Manager struct {
	ingresses  map[string]Ingress // ingressID -> Ingress
	configs    map[string]Config  // ingressID -> Config (for persistence)
	factories  map[string]Factory // type -> Factory
	store      *store.IndexStore
	raftNode   *raft.RaftNode
	logger     *zap.Logger
	configFile string
	mu         sync.RWMutex
}

// NewManager creates a new ingress manager
func NewManager(dataDir string, store *store.IndexStore, raftNode *raft.RaftNode, logger *zap.Logger) *Manager {
	return &Manager{
		ingresses:  make(map[string]Ingress),
		configs:    make(map[string]Config),
		factories:  make(map[string]Factory),
		store:      store,
		raftNode:   raftNode,
		logger:     logger,
		configFile: filepath.Join(dataDir, "ingresses.json"),
	}
}

// RegisterFactory registers a factory for a given ingress type
func (m *Manager) RegisterFactory(ingressType string, factory Factory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.factories[ingressType] = factory
}

// Load loads ingress configurations from disk and creates ingresses
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No config file yet
		}
		return fmt.Errorf("failed to read ingress config: %w", err)
	}

	var configs map[string]Config
	if err := sonic.Unmarshal(data, &configs); err != nil {
		return fmt.Errorf("failed to parse ingress config: %w", err)
	}

	m.configs = configs

	// Create ingresses from loaded configs
	for id, cfg := range configs {
		factory, ok := m.factories[cfg.Type]
		if !ok {
			m.logger.Warn("Unknown ingress type, skipping",
				zap.String("id", id),
				zap.String("type", cfg.Type))
			continue
		}

		ingress, err := factory(cfg, m.store, m.raftNode, m.logger)
		if err != nil {
			m.logger.Error("Failed to create ingress",
				zap.String("id", id),
				zap.Error(err))
			continue
		}

		m.ingresses[id] = ingress
	}

	return nil
}

// save persists ingress configurations to disk
func (m *Manager) save() error {
	data, err := sonic.ConfigDefault.MarshalIndent(m.configs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal ingress config: %w", err)
	}

	if err := os.WriteFile(m.configFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write ingress config: %w", err)
	}

	return nil
}

// Create creates a new ingress
func (m *Manager) Create(indexID string, ingressType string, id string, rawConfig json.RawMessage) (Ingress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if ingress already exists
	if _, exists := m.ingresses[id]; exists {
		return nil, fmt.Errorf("ingress %s already exists", id)
	}

	// Check if index exists
	if _, _, err := m.store.GetIndex(indexID); err != nil {
		return nil, fmt.Errorf("index %s not found", indexID)
	}

	// Get factory for type
	factory, ok := m.factories[ingressType]
	if !ok {
		return nil, fmt.Errorf("unknown ingress type: %s", ingressType)
	}

	cfg := Config{
		ID:      id,
		IndexID: indexID,
		Type:    ingressType,
		Config:  rawConfig,
	}

	// Create ingress
	ingress, err := factory(cfg, m.store, m.raftNode, m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create ingress: %w", err)
	}

	m.ingresses[id] = ingress
	m.configs[id] = cfg

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save ingress config", zap.Error(err))
	}

	return ingress, nil
}

// Get returns an ingress by ID
func (m *Manager) Get(id string) (Ingress, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ingress, ok := m.ingresses[id]
	if !ok {
		return nil, fmt.Errorf("ingress %s not found", id)
	}

	return ingress, nil
}

// List returns all ingresses for an index
func (m *Manager) List(indexID string) []Ingress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Ingress
	for _, ingress := range m.ingresses {
		if ingress.IndexID() == indexID {
			result = append(result, ingress)
		}
	}

	return result
}

// ListAll returns all ingresses
func (m *Manager) ListAll() []Ingress {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Ingress, 0, len(m.ingresses))
	for _, ingress := range m.ingresses {
		result = append(result, ingress)
	}

	return result
}

// Delete removes an ingress
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ingress, ok := m.ingresses[id]
	if !ok {
		return fmt.Errorf("ingress %s not found", id)
	}

	// Stop the ingress first
	if err := ingress.Stop(); err != nil {
		m.logger.Warn("Error stopping ingress during delete",
			zap.String("id", id),
			zap.Error(err))
	}

	delete(m.ingresses, id)
	delete(m.configs, id)

	if err := m.save(); err != nil {
		m.logger.Error("Failed to save ingress config", zap.Error(err))
	}

	return nil
}

// StartAll starts all ingresses
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ctx == nil {
		ctx = context.Background()
	}

	var firstErr error
	for id, ingress := range m.ingresses {
		if err := ingress.Start(ctx); err != nil {
			m.logger.Error("Failed to start ingress",
				zap.String("id", id),
				zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// StopAll stops all ingresses
func (m *Manager) StopAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var firstErr error
	for id, ingress := range m.ingresses {
		if err := ingress.Stop(); err != nil {
			m.logger.Error("Failed to stop ingress",
				zap.String("id", id),
				zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

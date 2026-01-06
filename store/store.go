package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"bright/models"

	"github.com/blevesearch/bleve/v2"
	"github.com/bytedance/sonic"
)

// IndexStore manages all indexes
type IndexStore struct {
	indexes    map[string]bleve.Index
	configs    map[string]*models.IndexConfig
	mu         sync.RWMutex
	dataDir    string
	configFile string
}

var store *IndexStore
var once sync.Once

// GetStore returns the singleton instance of IndexStore
func GetStore() *IndexStore {
	once.Do(func() {
		store = &IndexStore{
			indexes:    make(map[string]bleve.Index),
			configs:    make(map[string]*models.IndexConfig),
			dataDir:    "./data",
			configFile: "./data/configs.json",
		}
		store.loadConfigs()
	})
	return store
}

// CreateIndex creates a new bleve index
func (s *IndexStore) CreateIndex(config *models.IndexConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.indexes[config.ID]; exists {
		return fmt.Errorf("index %s already exists", config.ID)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	indexPath := filepath.Join(s.dataDir, config.ID)

	// Create index mapping
	indexMapping := bleve.NewIndexMapping()
	index, err := bleve.New(indexPath, indexMapping)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	s.indexes[config.ID] = index
	s.configs[config.ID] = config
	s.saveConfigs()

	return nil
}

// GetIndex returns an index by ID
func (s *IndexStore) GetIndex(id string) (bleve.Index, *models.IndexConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	index, exists := s.indexes[id]
	if !exists {
		return nil, nil, fmt.Errorf("index %s not found", id)
	}

	config := s.configs[id]
	return index, config, nil
}

// DeleteIndex deletes an index
func (s *IndexStore) DeleteIndex(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, exists := s.indexes[id]
	if !exists {
		return fmt.Errorf("index %s not found", id)
	}

	// Close the index
	if err := index.Close(); err != nil {
		return fmt.Errorf("failed to close index: %w", err)
	}

	// Delete the index directory
	indexPath := filepath.Join(s.dataDir, id)
	if err := os.RemoveAll(indexPath); err != nil {
		return fmt.Errorf("failed to delete index directory: %w", err)
	}

	delete(s.indexes, id)
	delete(s.configs, id)
	s.saveConfigs()

	return nil
}

// UpdateIndex updates index configuration
func (s *IndexStore) UpdateIndex(id string, config *models.IndexConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.indexes[id]; !exists {
		return fmt.Errorf("index %s not found", id)
	}

	config.ID = id // Ensure ID doesn't change
	s.configs[id] = config
	s.saveConfigs()

	return nil
}

// loadConfigs loads index configurations from disk
func (s *IndexStore) loadConfigs() {
	// Create data directory if it doesn't exist
	os.MkdirAll(s.dataDir, 0755)

	data, err := os.ReadFile(s.configFile)
	if err != nil {
		return // No configs to load
	}

	var configs map[string]*models.IndexConfig
	if err := sonic.Unmarshal(data, &configs); err != nil {
		return
	}

	s.configs = configs

	// Open existing indexes or recreate if missing
	for id := range configs {
		indexPath := filepath.Join(s.dataDir, id)

		// Check if index directory exists
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			// Index directory doesn't exist, recreate it
			indexMapping := bleve.NewIndexMapping()
			index, err := bleve.New(indexPath, indexMapping)
			if err != nil {
				continue
			}
			s.indexes[id] = index
		} else {
			// Index directory exists, try to open it
			index, err := bleve.Open(indexPath)
			if err != nil {
				// Failed to open, try to recreate
				os.RemoveAll(indexPath)
				indexMapping := bleve.NewIndexMapping()
				index, err = bleve.New(indexPath, indexMapping)
				if err != nil {
					continue
				}
			}
			s.indexes[id] = index
		}
	}
}

// saveConfigs saves index configurations to disk
func (s *IndexStore) saveConfigs() {
	data, err := sonic.ConfigDefault.MarshalIndent(s.configs, "", "  ")
	if err != nil {
		return
	}

	os.WriteFile(s.configFile, data, 0644)
}

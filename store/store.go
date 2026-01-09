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

// Initialize initializes the store with the specified data directory
// Must be called before GetStore() if you want to use a custom data directory
// Returns the initialized IndexStore
func Initialize(dataDir string) *IndexStore {
	once.Do(func() {
		store = &IndexStore{
			indexes:    make(map[string]bleve.Index),
			configs:    make(map[string]*models.IndexConfig),
			dataDir:    dataDir,
			configFile: filepath.Join(dataDir, "configs.json"),
		}
		store.loadConfigs()
	})
	return store
}

// GetStore returns the singleton instance of IndexStore
func GetStore() *IndexStore {
	once.Do(func() {
		// Default initialization if Initialize was not called
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

	// Apply exclude attributes if specified
	if len(config.ExcludeAttributes) > 0 {
		defaultMapping := indexMapping.DefaultMapping
		for _, attr := range config.ExcludeAttributes {
			disabledMapping := bleve.NewDocumentDisabledMapping()
			defaultMapping.AddSubDocumentMapping(attr, disabledMapping)
		}
	}

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

// ListIndexes returns all index configurations with pagination
func (s *IndexStore) ListIndexes(limit, offset int) []*models.IndexConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice
	allConfigs := make([]*models.IndexConfig, 0, len(s.configs))
	for _, config := range s.configs {
		allConfigs = append(allConfigs, config)
	}

	// Apply pagination
	start := offset
	if start > len(allConfigs) {
		return []*models.IndexConfig{}
	}

	end := start + limit
	if end > len(allConfigs) {
		end = len(allConfigs)
	}

	return allConfigs[start:end]
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

// GetAllConfigs returns all index configurations (for snapshotting)
func (s *IndexStore) GetAllConfigs() map[string]*models.IndexConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	configs := make(map[string]*models.IndexConfig, len(s.configs))
	for k, v := range s.configs {
		configs[k] = v
	}
	return configs
}

// RestoreConfigs restores index configurations from snapshot
func (s *IndexStore) RestoreConfigs(configs map[string]*models.IndexConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.configs = configs
	s.saveConfigs()
	return nil
}

// Internal methods (lock-free, called by FSM)

// CreateIndexInternal creates an index without locking (called by FSM)
func (s *IndexStore) CreateIndexInternal(config *models.IndexConfig) error {
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

	// Apply exclude attributes if specified
	if len(config.ExcludeAttributes) > 0 {
		defaultMapping := indexMapping.DefaultMapping
		for _, attr := range config.ExcludeAttributes {
			disabledMapping := bleve.NewDocumentDisabledMapping()
			defaultMapping.AddSubDocumentMapping(attr, disabledMapping)
		}
	}

	index, err := bleve.New(indexPath, indexMapping)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}

	s.indexes[config.ID] = index
	s.configs[config.ID] = config
	s.saveConfigs()

	return nil
}

// DeleteIndexInternal deletes an index without locking (called by FSM)
func (s *IndexStore) DeleteIndexInternal(id string) error {
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

// UpdateIndexInternal updates index configuration without locking (called by FSM)
func (s *IndexStore) UpdateIndexInternal(id string, config *models.IndexConfig) error {
	if _, exists := s.indexes[id]; !exists {
		return fmt.Errorf("index %s not found", id)
	}

	config.ID = id // Ensure ID doesn't change
	s.configs[id] = config
	s.saveConfigs()

	return nil
}

// AddDocumentsInternal adds documents to an index without locking (called by FSM)
func (s *IndexStore) AddDocumentsInternal(indexID string, documents []map[string]interface{}) error {
	index, exists := s.indexes[indexID]
	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	config := s.configs[indexID]
	batch := index.NewBatch()

	for _, doc := range documents {
		var docID string
		if id, ok := doc[config.PrimaryKey]; ok && id != nil {
			docID = fmt.Sprintf("%v", id)
		} else {
			return fmt.Errorf("document missing primary key %s", config.PrimaryKey)
		}

		if err := batch.Index(docID, doc); err != nil {
			return fmt.Errorf("failed to index document: %w", err)
		}
	}

	if err := index.Batch(batch); err != nil {
		return fmt.Errorf("failed to commit batch: %w", err)
	}

	return nil
}

// DeleteDocumentInternal deletes a document without locking (called by FSM)
func (s *IndexStore) DeleteDocumentInternal(indexID, documentID string) error {
	index, exists := s.indexes[indexID]
	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	if err := index.Delete(documentID); err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	return nil
}

// DeleteDocumentsInternal deletes multiple documents without locking (called by FSM)
func (s *IndexStore) DeleteDocumentsInternal(indexID, filter string, ids []string) error {
	index, exists := s.indexes[indexID]
	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	batch := index.NewBatch()

	// If specific IDs are provided
	if len(ids) > 0 {
		for _, id := range ids {
			batch.Delete(id)
		}
	} else if filter != "" {
		// Search with filter and delete matching documents
		query := bleve.NewQueryStringQuery(filter)
		searchRequest := bleve.NewSearchRequest(query)
		searchRequest.Size = 10000 // Limit for safety

		searchResult, err := index.Search(searchRequest)
		if err != nil {
			return fmt.Errorf("failed to search: %w", err)
		}

		for _, hit := range searchResult.Hits {
			batch.Delete(hit.ID)
		}
	} else {
		return fmt.Errorf("must provide ids or filter parameter to delete documents")
	}

	if err := index.Batch(batch); err != nil {
		return fmt.Errorf("failed to delete documents: %w", err)
	}

	return nil
}

// UpdateDocumentInternal updates a document without locking (called by FSM)
func (s *IndexStore) UpdateDocumentInternal(indexID, documentID string, updates map[string]interface{}) error {
	index, exists := s.indexes[indexID]
	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	// Get existing document by searching for it
	query := bleve.NewDocIDQuery([]string{documentID})
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Fields = []string{"*"}
	searchResult, err := index.Search(searchRequest)
	if err != nil || len(searchResult.Hits) == 0 {
		return fmt.Errorf("document not found")
	}

	// Merge updates with existing document
	existingData := make(map[string]interface{})
	if len(searchResult.Hits) > 0 {
		for fieldName, fieldValue := range searchResult.Hits[0].Fields {
			existingData[fieldName] = fieldValue
		}
	}

	for key, value := range updates {
		existingData[key] = value
	}

	// Re-index the document
	if err := index.Index(documentID, existingData); err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	return nil
}

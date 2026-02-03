package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"bright/models"

	"github.com/blevesearch/bleve/v2"
	"github.com/bytedance/sonic"
)

// IndexStore manages all indexes
type IndexStore struct {
	indexes    map[string]bleve.Index
	configs    map[string]*models.IndexConfig
	indexLocks map[string]*sync.RWMutex
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
			indexLocks: make(map[string]*sync.RWMutex),
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
			indexLocks: make(map[string]*sync.RWMutex),
			dataDir:    "./data",
			configFile: "./data/configs.json",
		}
		store.loadConfigs()
	})
	return store
}

// getIndexLock returns the lock for a specific index, creating it if necessary
func (s *IndexStore) getIndexLock(indexID string) *sync.RWMutex {
	s.mu.Lock()
	defer s.mu.Unlock()

	if lock, exists := s.indexLocks[indexID]; exists {
		return lock
	}

	lock := &sync.RWMutex{}
	s.indexLocks[indexID] = lock
	return lock
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

	var index bleve.Index
	var err error

	// Check if index directory already exists on disk
	if _, statErr := os.Stat(indexPath); statErr == nil {
		// Directory exists, try to open existing index
		index, err = bleve.Open(indexPath)
		if err != nil {
			// Failed to open, remove and recreate
			os.RemoveAll(indexPath)
			index, err = s.createNewIndex(indexPath, config)
			if err != nil {
				return err
			}
		}
	} else {
		// Directory doesn't exist, create new index
		index, err = s.createNewIndex(indexPath, config)
		if err != nil {
			return err
		}
	}

	s.indexes[config.ID] = index
	s.configs[config.ID] = config
	s.indexLocks[config.ID] = &sync.RWMutex{}
	s.saveConfigs()

	return nil
}

// createNewIndex creates a new bleve index with the given config
func (s *IndexStore) createNewIndex(indexPath string, config *models.IndexConfig) (bleve.Index, error) {
	indexMapping := bleve.NewIndexMapping()
	if len(config.ExcludeAttributes) > 0 {
		defaultMapping := indexMapping.DefaultMapping
		for _, attr := range config.ExcludeAttributes {
			disabledMapping := bleve.NewDocumentDisabledMapping()
			defaultMapping.AddSubDocumentMapping(attr, disabledMapping)
		}
	}
	index, err := bleve.New(indexPath, indexMapping)
	if err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}
	return index, nil
}

// GetIndex returns an index by ID
func (s *IndexStore) GetIndex(id string) (bleve.Index, *models.IndexConfig, error) {
	s.mu.RLock()
	index, exists := s.indexes[id]
	config := s.configs[id]
	s.mu.RUnlock()

	if !exists {
		return nil, nil, fmt.Errorf("index %s not found", id)
	}

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
	delete(s.indexLocks, id)
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
			s.indexLocks[id] = &sync.RWMutex{}
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
			s.indexLocks[id] = &sync.RWMutex{}
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

	var index bleve.Index
	var err error

	// Check if index directory already exists on disk
	if _, statErr := os.Stat(indexPath); statErr == nil {
		// Directory exists, try to open existing index
		index, err = bleve.Open(indexPath)
		if err != nil {
			// Failed to open, remove and recreate
			os.RemoveAll(indexPath)
			index, err = s.createNewIndex(indexPath, config)
			if err != nil {
				return err
			}
		}
	} else {
		// Directory doesn't exist, create new index
		index, err = s.createNewIndex(indexPath, config)
		if err != nil {
			return err
		}
	}

	s.indexes[config.ID] = index
	s.configs[config.ID] = config
	s.indexLocks[config.ID] = &sync.RWMutex{}
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
	delete(s.indexLocks, id)
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
func (s *IndexStore) AddDocumentsInternal(indexID string, documents []map[string]any) error {
	s.mu.RLock()
	index, exists := s.indexes[indexID]
	config := s.configs[indexID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	indexLock := s.getIndexLock(indexID)
	indexLock.Lock()
	defer indexLock.Unlock()

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
	s.mu.RLock()
	index, exists := s.indexes[indexID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	indexLock := s.getIndexLock(indexID)
	indexLock.Lock()
	defer indexLock.Unlock()

	if err := index.Delete(documentID); err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	return nil
}

// DeleteDocumentsInternal deletes multiple documents without locking (called by FSM)
func (s *IndexStore) DeleteDocumentsInternal(indexID, filter string, ids []string) error {
	s.mu.RLock()
	index, exists := s.indexes[indexID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	indexLock := s.getIndexLock(indexID)
	indexLock.Lock()
	defer indexLock.Unlock()

	batch := index.NewBatch()

	// If specific IDs are provided
	if len(ids) > 0 {
		for _, id := range ids {
			batch.Delete(id)
		}
	} else if filter != "" {
		// Search with filter and delete matching documents using pagination
		query := bleve.NewQueryStringQuery(filter)
		pageSize := 10000
		offset := 0

		for {
			searchRequest := bleve.NewSearchRequest(query)
			searchRequest.From = offset
			searchRequest.Size = pageSize

			searchResult, err := index.Search(searchRequest)
			if err != nil {
				return fmt.Errorf("failed to search: %w", err)
			}

			// If no results, we're done
			if len(searchResult.Hits) == 0 {
				break
			}

			// Delete documents from this page
			for _, hit := range searchResult.Hits {
				batch.Delete(hit.ID)
			}

			// If we got fewer results than page size, we've reached the end
			if len(searchResult.Hits) < pageSize {
				break
			}

			offset += pageSize
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
func (s *IndexStore) UpdateDocumentInternal(indexID, documentID string, updates map[string]any) error {
	s.mu.RLock()
	index, exists := s.indexes[indexID]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("index %s not found", indexID)
	}

	indexLock := s.getIndexLock(indexID)
	indexLock.Lock()
	defer indexLock.Unlock()

	// Get existing document by searching for it
	query := bleve.NewDocIDQuery([]string{documentID})
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Fields = []string{"*"}
	searchResult, err := index.Search(searchRequest)
	if err != nil || len(searchResult.Hits) == 0 {
		return fmt.Errorf("document not found")
	}

	// Merge updates with existing document
	existingData := make(map[string]any)
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

// DetectPrimaryKey analyzes documents and returns the primary key attribute
// Returns error if no candidates or multiple candidates are found
func DetectPrimaryKey(documents []map[string]any) (string, error) {
	if len(documents) == 0 {
		return "", fmt.Errorf("cannot detect primary key from empty document set")
	}

	// Collect all unique attribute names ending with "id" (case-insensitive)
	candidates := make(map[string]bool)

	for _, doc := range documents {
		for attr := range doc {
			if strings.HasSuffix(strings.ToLower(attr), "id") {
				candidates[attr] = true
			}
		}
	}

	// Validate exactly one candidate exists
	if len(candidates) == 0 {
		return "", fmt.Errorf("no primary key candidate found (no attribute ending with 'id')")
	}
	if len(candidates) > 1 {
		candidateList := make([]string, 0, len(candidates))
		for k := range candidates {
			candidateList = append(candidateList, k)
		}
		sort.Strings(candidateList)
		return "", fmt.Errorf("multiple primary key candidates found: %v", candidateList)
	}

	// Extract the single candidate
	for candidate := range candidates {
		return candidate, nil
	}

	return "", fmt.Errorf("unexpected error in primary key detection")
}

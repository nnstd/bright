package raft

import "encoding/json"

// CommandType represents the type of operation to be replicated
type CommandType string

const (
	// Index operations
	CommandCreateIndex CommandType = "create_index"
	CommandDeleteIndex CommandType = "delete_index"
	CommandUpdateIndex CommandType = "update_index"

	// Document operations
	CommandAddDocuments    CommandType = "add_documents"
	CommandDeleteDocument  CommandType = "delete_document"
	CommandDeleteDocuments CommandType = "delete_documents"
	CommandUpdateDocument  CommandType = "update_document"

	// Compound operations
	CommandAutoCreateAndAddDocuments CommandType = "auto_create_and_add_documents"
)

// Command represents a replicated operation that flows through Raft consensus
type Command struct {
	Type CommandType     `json:"type"`
	Data json.RawMessage `json:"data"`
}

// Index operation payloads

// CreateIndexPayload contains data for creating an index
type CreateIndexPayload struct {
	ID         string `json:"id"`
	PrimaryKey string `json:"primaryKey"`
}

// DeleteIndexPayload contains data for deleting an index
type DeleteIndexPayload struct {
	ID string `json:"id"`
}

// UpdateIndexPayload contains data for updating an index configuration
type UpdateIndexPayload struct {
	ID         string `json:"id"`
	PrimaryKey string `json:"primaryKey"`
}

// Document operation payloads

// AddDocumentsPayload contains data for adding documents to an index
type AddDocumentsPayload struct {
	IndexID   string           `json:"index_id"`
	Documents []map[string]any `json:"documents"`
}

// DeleteDocumentPayload contains data for deleting a single document
type DeleteDocumentPayload struct {
	IndexID    string `json:"index_id"`
	DocumentID string `json:"document_id"`
}

// DeleteDocumentsPayload contains data for deleting multiple documents
type DeleteDocumentsPayload struct {
	IndexID string   `json:"index_id"`
	Filter  string   `json:"filter"`
	IDs     []string `json:"ids"`
}

// UpdateDocumentPayload contains data for updating a document
type UpdateDocumentPayload struct {
	IndexID    string         `json:"index_id"`
	DocumentID string         `json:"document_id"`
	Updates    map[string]any `json:"updates"`
}

// AutoCreateAndAddDocumentsPayload contains data for auto-creating an index and adding documents
type AutoCreateAndAddDocumentsPayload struct {
	IndexID    string           `json:"index_id"`
	PrimaryKey string           `json:"primary_key"`
	Documents  []map[string]any `json:"documents"`
}

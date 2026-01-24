package handlers

import (
	"bright/errors"
	"bright/formats"
	"bright/models"
	"bright/raft"
	"bright/rpc"
	"bright/store"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// handleRaftAutoCreate handles automatic index creation in Raft mode
func handleRaftAutoCreate(c *fiber.Ctx, indexID string, config *models.IndexConfig, documents []map[string]interface{}) error {
	ctx := GetContext(c)

	if !IsLeader(c) {
		return rpc.ForwardToLeader(c, ctx.RPCClient, ctx.RaftNode.LeaderAddr())
	}

	// Generate UUIDs for documents missing primary key
	for _, doc := range documents {
		if id, ok := doc[config.PrimaryKey]; !ok || id == nil {
			uuidV7, err := uuid.NewV7()
			if err != nil {
				return errors.InternalError(c, errors.ErrorCodeUUIDGenerationFailed, "failed to generate UUID")
			}
			doc[config.PrimaryKey] = uuidV7.String()
		}
	}

	// Serialize payload
	payloadData, err := sonic.Marshal(raft.AutoCreateAndAddDocumentsPayload{
		IndexID:    indexID,
		PrimaryKey: config.PrimaryKey,
		Documents:  documents,
	})
	if err != nil {
		return errors.InternalErrorWithDetails(c, errors.ErrorCodeSerializationFailed, "failed to serialize payload", err.Error())
	}

	// Apply via Raft
	cmd := raft.Command{
		Type: raft.CommandAutoCreateAndAddDocuments,
		Data: json.RawMessage(payloadData),
	}

	if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
		return errors.InternalErrorWithDetails(c, errors.ErrorCodeRaftApplyFailed, "failed to auto-create index and add documents", err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"indexed":      len(documents),
		"auto_created": true,
		"primary_key":  config.PrimaryKey,
	})
}

// AddDocuments handles POST /indexes/:id/documents
func AddDocuments(c *fiber.Ctx) error {
	indexID := c.Params("id")
	format := c.Query("format", "jsoneachrow")
	primaryKey := c.Query("primaryKey")

	body := c.Body()

	// Get the appropriate parser for the format
	parser, err := formats.GetParser(format)
	if err != nil {
		return errors.BadRequestWithDetails(c, errors.ErrorCodeInvalidFormat, "invalid format parameter", err.Error())
	}

	// Parse documents using the format parser
	documents, err := parser.Parse(body)
	if err != nil {
		return errors.BadRequestWithDetails(c, errors.ErrorCodeParseError, "failed to parse documents", err.Error())
	}

	s := store.GetStore()
	index, config, err := s.GetIndex(indexID)

	// If index doesn't exist, attempt auto-creation if enabled
	if err != nil {
		ctx := GetContext(c)
		if !ctx.Config.AutoCreateIndex {
			return errors.NotFound(c, errors.ErrorCodeIndexNotFound, err.Error())
		}

		// Use provided primaryKey or detect from documents
		var detectedPrimaryKey string
		if primaryKey != "" {
			detectedPrimaryKey = primaryKey
		} else {
			var err error
			detectedPrimaryKey, err = store.DetectPrimaryKey(documents)
			if err != nil {
				return errors.BadRequestWithDetails(c, errors.ErrorCodeInvalidParameter, "cannot auto-create index", err.Error())
			}
		}

		autoConfig := &models.IndexConfig{
			ID:         indexID,
			PrimaryKey: detectedPrimaryKey,
		}

		// Single-node mode: create directly
		if !IsRaftEnabled(c) {
			if err := s.CreateIndex(autoConfig); err != nil {
				return errors.InternalErrorWithDetails(c, errors.ErrorCodeIndexOperationFailed, "failed to auto-create index", err.Error())
			}
			// Get the newly created index
			index, config, err = s.GetIndex(indexID)
			if err != nil {
				return errors.InternalError(c, errors.ErrorCodeIndexOperationFailed, err.Error())
			}
		} else {
			// Raft mode: use compound command
			return handleRaftAutoCreate(c, indexID, autoConfig, documents)
		}
	}

	// Determine which primary key to use
	effectivePrimaryKey := config.PrimaryKey
	if primaryKey != "" {
		effectivePrimaryKey = primaryKey
	}

	// Generate document IDs for documents that don't have one
	for _, doc := range documents {
		if id, ok := doc[effectivePrimaryKey]; !ok || id == nil {
			// Generate UUID v7
			uuidV7, err := uuid.NewV7()
			if err != nil {
				return errors.InternalError(c, errors.ErrorCodeUUIDGenerationFailed, "failed to generate UUID")
			}
			doc[effectivePrimaryKey] = uuidV7.String()
		}
	}

	ctx := GetContext(c)

	// If Raft is enabled, apply command through consensus
	if IsRaftEnabled(c) {
		if !IsLeader(c) {
			// Forward to leader
			return rpc.ForwardToLeader(c, ctx.RPCClient, ctx.RaftNode.LeaderAddr())
		}

		// Serialize payload
		payloadData, err := sonic.Marshal(raft.AddDocumentsPayload{
			IndexID:   indexID,
			Documents: documents,
		})
		if err != nil {
			return errors.InternalErrorWithDetails(c, errors.ErrorCodeSerializationFailed, "failed to serialize payload", err.Error())
		}

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandAddDocuments,
			Data: json.RawMessage(payloadData),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return errors.InternalErrorWithDetails(c, errors.ErrorCodeRaftApplyFailed, "failed to add documents via Raft", err.Error())
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"indexed": len(documents),
		})
	}

	// Single-node mode: process each document in a batch
	batch := index.NewBatch()
	for _, doc := range documents {
		var docID string
		if id, ok := doc[effectivePrimaryKey]; ok && id != nil {
			docID = fmt.Sprintf("%v", id)
		} else {
			return errors.InternalError(c, errors.ErrorCodeDocumentOperationFailed, "document missing primary key")
		}

		// Index or update the document
		if err := batch.Index(docID, doc); err != nil {
			return errors.InternalErrorWithDetails(c, errors.ErrorCodeDocumentOperationFailed, "failed to index document", err.Error())
		}
	}

	if err := index.Batch(batch); err != nil {
		return errors.InternalErrorWithDetails(c, errors.ErrorCodeBatchOperationFailed, "failed to commit batch", err.Error())
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"indexed": len(documents),
	})
}

// DeleteDocuments handles DELETE /indexes/:id/documents
func DeleteDocuments(c *fiber.Ctx) error {
	indexID := c.Params("id")

	// Parse query parameters
	var params struct {
		Filter string   `query:"filter"`
		IDs    []string `query:"ids[]"`
	}

	if err := c.QueryParser(&params); err != nil {
		return errors.BadRequestWithDetails(c, errors.ErrorCodeInvalidParameter, "invalid query parameters", err.Error())
	}

	filter := params.Filter
	idsStr := params.IDs

	s := store.GetStore()
	index, _, err := s.GetIndex(indexID)
	if err != nil {
		return errors.NotFound(c, errors.ErrorCodeIndexNotFound, err.Error())
	}

	batch := index.NewBatch()

	// If specific IDs are provided
	if len(idsStr) > 0 {
		for _, id := range idsStr {
			batch.Delete(id)
		}
	} else if filter != "" {
		// Search with filter and delete matching documents
		query := bleve.NewQueryStringQuery(filter)
		searchRequest := bleve.NewSearchRequest(query)
		searchRequest.Size = 10000 // Limit for safety

		searchResult, err := index.Search(searchRequest)
		if err != nil {
			return errors.BadRequestWithDetails(c, errors.ErrorCodeSearchFailed, "failed to search documents", err.Error())
		}

		for _, hit := range searchResult.Hits {
			batch.Delete(hit.ID)
		}
	} else {
		// Delete all documents - recreate the index
		return errors.BadRequest(c, errors.ErrorCodeMissingParameter, "must provide ids[] or filter parameter to delete documents")
	}

	if err := index.Batch(batch); err != nil {
		return errors.InternalErrorWithDetails(c, errors.ErrorCodeBatchOperationFailed, "failed to delete documents", err.Error())
	}

	return c.Status(fiber.StatusNoContent).Send(nil)
}

// DeleteDocument handles DELETE /indexes/:id/documents/:documentid
func DeleteDocument(c *fiber.Ctx) error {
	indexID := c.Params("id")
	documentID := c.Params("documentid")

	s := store.GetStore()
	index, _, err := s.GetIndex(indexID)
	if err != nil {
		return errors.NotFound(c, errors.ErrorCodeIndexNotFound, err.Error())
	}

	if err := index.Delete(documentID); err != nil {
		return errors.InternalErrorWithDetails(c, errors.ErrorCodeDocumentOperationFailed, "failed to delete document", err.Error())
	}

	return c.Status(fiber.StatusNoContent).Send(nil)
}

// UpdateDocument handles PATCH /indexes/:id/documents/:documentid
func UpdateDocument(c *fiber.Ctx) error {
	indexID := c.Params("id")
	documentID := c.Params("documentid")

	s := store.GetStore()
	index, _, err := s.GetIndex(indexID)
	if err != nil {
		return errors.NotFound(c, errors.ErrorCodeIndexNotFound, err.Error())
	}

	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return errors.BadRequest(c, errors.ErrorCodeInvalidRequestBody, "invalid request body")
	}

	// Get existing document
	query := bleve.NewDocIDQuery([]string{documentID})
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Fields = []string{"*"}
	searchResult, err := index.Search(searchRequest)
	if err != nil || len(searchResult.Hits) == 0 {
		return errors.NotFound(c, errors.ErrorCodeDocumentNotFound, "document not found")
	}

	// Merge updates with existing document
	existingData := make(map[string]interface{})
	for fieldName, fieldValue := range searchResult.Hits[0].Fields {
		existingData[fieldName] = fieldValue
	}

	for key, value := range updates {
		existingData[key] = value
	}

	// Re-index the document
	if err := index.Index(documentID, existingData); err != nil {
		return errors.InternalErrorWithDetails(c, errors.ErrorCodeDocumentOperationFailed, "failed to update document", err.Error())
	}

	return c.JSON(existingData)
}

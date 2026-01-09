package handlers

import (
	"bright/formats"
	"bright/models"
	"bright/raft"
	"bright/store"
	"encoding/json"
	"fmt"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// handleRaftAutoCreate handles automatic index creation in Raft mode
func handleRaftAutoCreate(c *fiber.Ctx, indexID string, config *models.IndexConfig, documents []map[string]interface{}) error {
	ctx := GetContext(c)

	if !IsLeader(c) {
		return c.Status(fiber.StatusTemporaryRedirect).JSON(fiber.Map{
			"error":  "not leader",
			"leader": ctx.RaftNode.LeaderAddr(),
		})
	}

	// Generate UUIDs for documents missing primary key
	for _, doc := range documents {
		if id, ok := doc[config.PrimaryKey]; !ok || id == nil {
			uuidV7, err := uuid.NewV7()
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "failed to generate UUID",
				})
			}
			doc[config.PrimaryKey] = uuidV7.String()
		}
	}

	// Serialize payload
	payloadData, err := json.Marshal(raft.AutoCreateAndAddDocumentsPayload{
		IndexID:    indexID,
		PrimaryKey: config.PrimaryKey,
		Documents:  documents,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to serialize payload: %v", err),
		})
	}

	// Apply via Raft
	cmd := raft.Command{
		Type: raft.CommandAutoCreateAndAddDocuments,
		Data: json.RawMessage(payloadData),
	}

	if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
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

	body := c.Body()

	// Get the appropriate parser for the format
	parser, err := formats.GetParser(format)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Parse documents using the format parser
	documents, err := parser.Parse(body)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("parse error: %v", err),
		})
	}

	s := store.GetStore()
	index, config, err := s.GetIndex(indexID)

	// If index doesn't exist, attempt auto-creation if enabled
	if err != nil {
		ctx := GetContext(c)
		if !ctx.Config.AutoCreateIndex {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		// Detect primary key from documents
		primaryKey, err := store.DetectPrimaryKey(documents)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("cannot auto-create index: %v", err),
			})
		}

		autoConfig := &models.IndexConfig{
			ID:         indexID,
			PrimaryKey: primaryKey,
		}

		// Single-node mode: create directly
		if !IsRaftEnabled(c) {
			if err := s.CreateIndex(autoConfig); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": fmt.Sprintf("failed to auto-create index: %v", err),
				})
			}
			// Get the newly created index
			index, config, err = s.GetIndex(indexID)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": err.Error(),
				})
			}
		} else {
			// Raft mode: use compound command
			return handleRaftAutoCreate(c, indexID, autoConfig, documents)
		}
	}

	// Generate document IDs for documents that don't have one
	for _, doc := range documents {
		if id, ok := doc[config.PrimaryKey]; !ok || id == nil {
			// Generate UUID v7
			uuidV7, err := uuid.NewV7()
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "failed to generate UUID",
				})
			}
			doc[config.PrimaryKey] = uuidV7.String()
		}
	}

	ctx := GetContext(c)

	// If Raft is enabled, apply command through consensus
	if IsRaftEnabled(c) {
		if !IsLeader(c) {
			// Redirect to leader
			return c.Status(fiber.StatusTemporaryRedirect).JSON(fiber.Map{
				"error":  "not leader",
				"leader": ctx.RaftNode.LeaderAddr(),
			})
		}

		// Serialize payload
		payloadData, err := json.Marshal(raft.AddDocumentsPayload{
			IndexID:   indexID,
			Documents: documents,
		})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("failed to serialize payload: %v", err),
			})
		}

		// Apply command via Raft
		cmd := raft.Command{
			Type: raft.CommandAddDocuments,
			Data: json.RawMessage(payloadData),
		}

		if err := ctx.RaftNode.Apply(cmd, 10*time.Second); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"indexed": len(documents),
		})
	}

	// Single-node mode: process each document in a batch
	batch := index.NewBatch()
	for _, doc := range documents {
		var docID string
		if id, ok := doc[config.PrimaryKey]; ok && id != nil {
			docID = fmt.Sprintf("%v", id)
		} else {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "document missing primary key",
			})
		}

		// Index or update the document
		if err := batch.Index(docID, doc); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": fmt.Sprintf("failed to index document: %v", err),
			})
		}
	}

	if err := index.Batch(batch); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to commit batch: %v", err),
		})
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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": fmt.Sprintf("invalid query parameters: %v", err),
		})
	}

	filter := params.Filter
	idsStr := params.IDs

	s := store.GetStore()
	index, _, err := s.GetIndex(indexID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
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
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fmt.Sprintf("failed to search: %v", err),
			})
		}

		for _, hit := range searchResult.Hits {
			batch.Delete(hit.ID)
		}
	} else {
		// Delete all documents - recreate the index
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "must provide ids[] or filter parameter to delete documents",
		})
	}

	if err := index.Batch(batch); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to delete documents: %v", err),
		})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if err := index.Delete(documentID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to delete document: %v", err),
		})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	var updates map[string]interface{}
	if err := c.BodyParser(&updates); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	// Get existing document by searching for it
	query := bleve.NewDocIDQuery([]string{documentID})
	searchRequest := bleve.NewSearchRequest(query)
	searchRequest.Fields = []string{"*"}
	searchResult, err := index.Search(searchRequest)
	if err != nil || len(searchResult.Hits) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "document not found",
		})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("failed to update document: %v", err),
		})
	}

	return c.JSON(existingData)
}

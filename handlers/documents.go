package handlers

import (
	"bright/store"
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// AddDocuments handles POST /indexes/:id/documents
func AddDocuments(c *fiber.Ctx) error {
	indexID := c.Params("id")
	format := c.Query("format", "jsoneachrow")

	s := store.GetStore()
	index, config, err := s.GetIndex(indexID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	body := c.Body()

	var documents []map[string]interface{}

	if format == "jsoneachrow" {
		scanner := bufio.NewScanner(bytes.NewReader(body))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}

			var doc map[string]interface{}
			if err := sonic.Unmarshal([]byte(line), &doc); err != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error": fmt.Sprintf("invalid JSON: %v", err),
				})
			}
			documents = append(documents, doc)
		}
	} else {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "unsupported format",
		})
	}

	// Process each document
	batch := index.NewBatch()
	for _, doc := range documents {
		var docID string

		// Check if document has an ID
		if id, ok := doc[config.PrimaryKey]; ok && id != nil {
			docID = fmt.Sprintf("%v", id)
		} else {
			// Generate UUID v7
			uuidV7, err := uuid.NewV7()
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "failed to generate UUID",
				})
			}
			docID = uuidV7.String()
			doc[config.PrimaryKey] = docID
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

package handlers

import (
	"bright/errors"
	"bright/models"
	"bright/store"
	"math"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/gofiber/fiber/v2"
)

// Search handles POST /indexes/:id/searches
func Search(c *fiber.Ctx) error {
	indexID := c.Params("id")

	// Parse query parameters using struct
	var params struct {
		Q                    string   `query:"q"`
		Offset               int      `query:"offset"`
		Limit                int      `query:"limit"`
		Page                 int      `query:"page"`
		Sort                 []string `query:"sort[]"`
		AttributesToRetrieve []string `query:"attributesToRetrieve[]"`
		AttributesToExclude  []string `query:"attributesToExclude[]"`
	}

	// Set defaults
	params.Limit = 20
	params.Page = 1

	if err := c.QueryParser(&params); err != nil {
		return errors.BadRequestWithDetails(c, errors.ErrorCodeInvalidParameter, "invalid query parameters", err.Error())
	}

	// Parse request body if provided (can override query params)
	var bodyParams models.SearchRequest
	if err := c.BodyParser(&bodyParams); err == nil {
		// Override with body params if provided
		if bodyParams.Query != "" {
			params.Q = bodyParams.Query
		}
		if bodyParams.Limit > 0 {
			params.Limit = bodyParams.Limit
		}
		if bodyParams.Offset > 0 {
			params.Offset = bodyParams.Offset
		}
		if bodyParams.Page > 0 {
			params.Page = bodyParams.Page
		}
		if len(bodyParams.Sort) > 0 {
			params.Sort = bodyParams.Sort
		}
		if len(bodyParams.AttributesToRetrieve) > 0 {
			params.AttributesToRetrieve = bodyParams.AttributesToRetrieve
		}
		if len(bodyParams.AttributesToExclude) > 0 {
			params.AttributesToExclude = bodyParams.AttributesToExclude
		}
	}

	queryStr := params.Q
	offset := params.Offset
	limit := params.Limit
	sortFields := params.Sort
	page := params.Page
	attributesToRetrieve := params.AttributesToRetrieve
	attributesToExclude := params.AttributesToExclude

	// Validate that both attributesToRetrieve and attributesToExclude are not provided
	if len(attributesToRetrieve) > 0 && len(attributesToExclude) > 0 {
		return errors.BadRequest(c, errors.ErrorCodeConflictingParameters, "cannot use both attributesToRetrieve and attributesToExclude at the same time")
	}

	// Calculate offset from page if page is provided
	if page > 1 {
		offset = (page - 1) * limit
	}

	s := store.GetStore()
	index, _, err := s.GetIndex(indexID)
	if err != nil {
		return errors.NotFound(c, errors.ErrorCodeIndexNotFound, err.Error())
	}

	// Create search query
	var searchQuery query.Query
	if queryStr == "" {
		searchQuery = bleve.NewMatchAllQuery()
	} else {
		searchQuery = bleve.NewQueryStringQuery(queryStr)
	}

	searchRequest := bleve.NewSearchRequest(searchQuery)
	searchRequest.From = offset
	searchRequest.Size = limit

	// Optimize field retrieval: only request fields we need
	if len(attributesToRetrieve) > 0 {
		// Request only specified fields
		searchRequest.Fields = attributesToRetrieve
	} else if len(attributesToExclude) > 0 {
		// Request all fields (we'll exclude in post-processing)
		searchRequest.Fields = []string{"*"}
	} else {
		// Default: request all fields
		searchRequest.Fields = []string{"*"}
	}

	// Apply sorting if provided
	if len(sortFields) > 0 {
		sortOrder := make([]string, 0, len(sortFields))
		for _, sortField := range sortFields {
			sortField = strings.TrimSpace(sortField)
			if sortField != "" {
				// Check if field has descending order (starts with -)
				if strings.HasPrefix(sortField, "-") {
					// Descending order
					fieldName := strings.TrimPrefix(sortField, "-")
					sortOrder = append(sortOrder, "-"+fieldName)
				} else {
					// Ascending order (default)
					sortOrder = append(sortOrder, sortField)
				}
			}
		}

		if len(sortOrder) > 0 {
			searchRequest.SortBy(sortOrder)
		}
	} else {
		// Default sorting by score (relevance)
		searchRequest.SortBy([]string{"-_score"})
	}

	// Execute search
	searchResult, err := index.Search(searchRequest)
	if err != nil {
		return errors.BadRequestWithDetails(c, errors.ErrorCodeSearchFailed, "search failed", err.Error())
	}

	// Process results
	hits := make([]map[string]any, 0, len(searchResult.Hits))
	for _, hit := range searchResult.Hits {
		doc := make(map[string]any)

		// Add all fields from the hit
		for fieldName, fieldValue := range hit.Fields {
			doc[fieldName] = fieldValue
		}

		// Add the document ID
		if _, ok := doc["id"]; !ok {
			doc["id"] = hit.ID
		}

		// Apply attributesToExclude if specified (only needed when not using attributesToRetrieve)
		if len(attributesToExclude) > 0 {
			for _, attr := range attributesToExclude {
				delete(doc, attr)
			}
		}

		hits = append(hits, doc)
	}

	// Calculate total pages
	totalPages := int(math.Ceil(float64(searchResult.Total) / float64(limit)))

	response := models.SearchResponse{
		Hits:       hits,
		TotalHits:  searchResult.Total,
		TotalPages: totalPages,
	}

	return c.JSON(response)
}

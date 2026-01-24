package models

// IndexConfig represents the configuration for an index
type IndexConfig struct {
	ID                string   `json:"id"`
	PrimaryKey        string   `json:"primaryKey"`
	ExcludeAttributes []string `json:"excludeAttributes,omitempty"`
}

// SearchRequest represents a search request
type SearchRequest struct {
	Query                string   `json:"q"`
	Offset               int      `json:"offset"`
	Limit                int      `json:"limit"`
	Page                 int      `json:"page"`
	Sort                 []string `json:"sort,omitempty"`
	AttributesToRetrieve []string `json:"attributesToRetrieve"`
	AttributesToExclude  []string `json:"attributesToExclude"`
}

// SearchResponse represents a search response
type SearchResponse struct {
	Hits       []map[string]any `json:"hits"`
	TotalHits  uint64           `json:"totalHits"`
	TotalPages int              `json:"totalPages"`
}

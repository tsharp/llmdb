package main

import "time"

// Document represents a stored document with metadata
type Document struct {
	ID         string                 `json:"id"`
	DB         string                 `json:"db"`
	Table      string                 `json:"table"`
	Content    string                 `json:"content"` // Markdown text
	Metadata   map[string]interface{} `json:"metadata"`
	Tags       []string               `json:"tags,omitempty"`
	Vector     []float32              `json:"vector,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	IsEmbedded bool                   `json:"is_embedded"`
}

// StoreDocumentRequest represents the request to store a document
type StoreDocumentRequest struct {
	ID       string                 `json:"id"` // Optional, generated if not provided
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Tags     []string               `json:"tags,omitempty"`
}

// SearchRequest represents a search query
type SearchRequest struct {
	Query   string                 `json:"query"`
	Type    SearchType             `json:"type"` // "vector", "fulltext", or "hybrid"
	Limit   int                    `json:"limit,omitempty"`
	Filters map[string]interface{} `json:"filters,omitempty"`
}

// SearchType defines the type of search to perform
type SearchType string

const (
	SearchTypeVector   SearchType = "vector"
	SearchTypeFullText SearchType = "fulltext"
	SearchTypeHybrid   SearchType = "hybrid"
)

// SearchResult represents a single search result
type SearchResult struct {
	Document Document `json:"document"`
	Score    float64  `json:"score"`
	Rank     int      `json:"rank,omitempty"`
}

// SearchResponse represents the search results
type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Query   string         `json:"query"`
	Type    SearchType     `json:"type"`
	DB      string         `json:"db"`
	Total   int            `json:"total"`
}

// DBInfo represents information about a database
type DBInfo struct {
	Name          string    `json:"name"`
	DocumentCount int       `json:"document_count"`
	EmbeddedCount int       `json:"embedded_count"`
	CreatedAt     time.Time `json:"created_at"`
	LastUpdated   time.Time `json:"last_updated"`
	SizeBytes     int64     `json:"size_bytes"`
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

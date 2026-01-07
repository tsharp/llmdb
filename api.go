package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

const (
	ClientFeatures = "X-Client-Features"
)

// API handles HTTP requests
type API struct {
	store    *DocumentStore
	embedder Embedder
}

// NewAPI creates a new API instance
func NewAPI(store *DocumentStore, embedder Embedder) *API {
	return &API{
		store:    store,
		embedder: embedder,
	}
}

// StoreDocument creates or updates a document in a database table
// POST /db/{dbName}/{tableName}
func (a *API) StoreDocument(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["dbName"]
	tableName := vars["tableName"]

	if dbName == "" {
		a.errorResponse(w, http.StatusBadRequest, "database name is required")
		return
	}

	if tableName == "" {
		a.errorResponse(w, http.StatusBadRequest, "table name is required")
		return
	}

	var req StoreDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Content == "" {
		a.errorResponse(w, http.StatusBadRequest, "content is required")
		return
	}

	doc := &Document{
		ID:       req.ID,
		Content:  req.Content,
		Metadata: req.Metadata,
	}

	// Check if immediate vectorization is requested
	shouldEmbed := r.Header.Get(ClientFeatures) == "true"
	if shouldEmbed {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		vector, err := a.embedder.Embed(ctx, req.Content)
		if err != nil {
			a.errorResponse(w, http.StatusInternalServerError,
				fmt.Sprintf("vectorization failed: %v", err))
			return
		}
		doc.Vector = vector
		doc.IsVectorized = true
	}

	if err := a.store.StoreDocument(dbName, tableName, doc); err != nil {
		a.errorResponse(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to store document: %v", err))
		return
	}

	// Non-vectorized documents will be picked up by the background worker

	// Return minimal response by default
	schema := r.URL.Query().Get("schema")
	if schema != "full" {
		w.Header().Set("X-Schema", "minimal")
		minimalResp := map[string]interface{}{
			"id":            doc.ID,
			"created_at":    doc.CreatedAt,
			"updated_at":    doc.UpdatedAt,
			"is_vectorized": doc.IsVectorized,
		}
		a.jsonResponse(w, http.StatusCreated, minimalResp)
		return
	}

	// Full response
	w.Header().Set("X-Schema", "full")
	a.jsonResponse(w, http.StatusCreated, doc)
}

// GetDocument retrieves a document by ID
// GET /db/{dbName}/{tableName}/{docId}
func (a *API) GetDocument(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["dbName"]
	tableName := vars["tableName"]
	docId := vars["docId"]

	doc, err := a.store.GetDocument(dbName, tableName, docId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			a.errorResponse(w, http.StatusNotFound, "document not found")
		} else {
			a.errorResponse(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	a.jsonResponse(w, http.StatusOK, doc)
}

// DeleteDocument deletes a document by ID
// DELETE /db/{dbName}/{tableName}/{docId}
func (a *API) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["dbName"]
	tableName := vars["tableName"]
	docId := vars["docId"]

	if err := a.store.DeleteDocument(dbName, tableName, docId); err != nil {
		if strings.Contains(err.Error(), "not found") {
			a.errorResponse(w, http.StatusNotFound, "document not found")
		} else {
			a.errorResponse(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SearchDocuments searches documents within a database table
// POST /db/{dbName}/{tableName}/search
func (a *API) SearchDocuments(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["dbName"]
	tableName := vars["tableName"]

	var req SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.errorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Query == "" {
		a.errorResponse(w, http.StatusBadRequest, "query is required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 10
	}

	var results []SearchResult
	var err error

	switch req.Type {
	case SearchTypeFullText, "":
		// Default to full-text search
		results, err = a.store.SearchFullText(dbName, tableName, req.Query, req.Limit)
		if err != nil {
			a.errorResponse(w, http.StatusInternalServerError,
				fmt.Sprintf("full-text search failed: %v", err))
			return
		}

	case SearchTypeVector:
		// Convert query to vector
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		queryVector, err := a.embedder.Embed(ctx, req.Query)
		if err != nil {
			a.errorResponse(w, http.StatusInternalServerError,
				fmt.Sprintf("failed to vectorize query: %v", err))
			return
		}

		results, err = a.store.SearchVector(dbName, tableName, queryVector, req.Limit)
		if err != nil {
			a.errorResponse(w, http.StatusInternalServerError,
				fmt.Sprintf("vector search failed: %v", err))
			return
		}

	case SearchTypeHybrid:
		// TODO: Implement hybrid search (combine full-text + vector)
		// 1. Do full-text search to get candidates
		// 2. Re-rank using vector similarity
		// 3. Merge results
		a.errorResponse(w, http.StatusNotImplemented, "hybrid search not yet implemented")
		return

	default:
		a.errorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("invalid search type: %s", req.Type))
		return
	}

	// Add ranks to results
	for i := range results {
		results[i].Rank = i + 1
	}

	response := SearchResponse{
		Results: results,
		Query:   req.Query,
		Type:    req.Type,
		DB:      dbName,
		Total:   len(results),
	}

	a.jsonResponse(w, http.StatusOK, response)
}

// ListDatabases lists all available databases
// GET /db
func (a *API) ListDatabases(w http.ResponseWriter, r *http.Request) {
	databases, err := a.store.ListDatabases()
	if err != nil {
		a.errorResponse(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to list databases: %v", err))
		return
	}

	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"databases": databases,
		"count":     len(databases),
	})
}

// ListTables lists all tables in a database
// GET /db/{dbName}
func (a *API) ListTables(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["dbName"]

	tables, err := a.store.ListTables(dbName)
	if err != nil {
		a.errorResponse(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to list tables: %v", err))
		return
	}

	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"database": dbName,
		"tables":   tables,
		"count":    len(tables),
	})
}

// ListDocuments lists all documents in a database table
// GET /db/{dbName}/{tableName}
func (a *API) ListDocuments(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["dbName"]
	tableName := vars["tableName"]

	// Get optional query parameters for pagination
	limit := 100 // default
	offset := 0  // default

	// Check schema parameter - "full" for complete documents, default is minimal
	schema := r.URL.Query().Get("schema")
	fullSchema := schema == "full"

	documents, err := a.store.ListDocuments(dbName, tableName, limit, offset)
	if err != nil {
		a.errorResponse(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to list documents: %v", err))
		return
	}

	// Return minimal response by default (just IDs and metadata)
	if !fullSchema {
		type MinimalDoc struct {
			ID           string    `json:"id"`
			CreatedAt    time.Time `json:"created_at"`
			UpdatedAt    time.Time `json:"updated_at"`
			IsVectorized bool      `json:"is_vectorized"`
		}

		minimalDocs := make([]MinimalDoc, len(documents))
		for i, doc := range documents {
			minimalDocs[i] = MinimalDoc{
				ID:           doc.ID,
				CreatedAt:    doc.CreatedAt,
				UpdatedAt:    doc.UpdatedAt,
				IsVectorized: doc.IsVectorized,
			}
		}

		w.Header().Set("X-Schema", "minimal")
		a.jsonResponse(w, http.StatusOK, map[string]interface{}{
			"documents": minimalDocs,
			"count":     len(minimalDocs),
			"limit":     limit,
			"offset":    offset,
		})
		return
	}

	// Full schema - return complete documents
	w.Header().Set("X-Schema", "full")
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"documents": documents,
		"count":     len(documents),
		"limit":     limit,
		"offset":    offset,
	})
}

// DeleteDatabase deletes an entire database
// DELETE /db/{dbName}
func (a *API) DeleteDatabase(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	dbName := vars["dbName"]

	if err := a.store.DeleteDatabase(dbName); err != nil {
		a.errorResponse(w, http.StatusInternalServerError,
			fmt.Sprintf("failed to delete database: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Health returns the health status of the service
// GET /health
func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	a.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"service":   "context-pipeline",
	})
}

// Helper methods

func (a *API) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (a *API) errorResponse(w http.ResponseWriter, status int, message string) {
	a.jsonResponse(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}

// Background vectorization queue (stub)
// TODO: Implement background job processing
// Options:
// 1. In-memory queue with worker goroutines
// 2. Redis queue with worker processes
// 3. Database-backed queue (polling)
//
// func (a *API) queueVectorization(partition, docID string) error {
//     // Add to queue for background processing
//     return nil
// }
//
// func (a *API) startBackgroundWorkers(ctx context.Context, numWorkers int) {
//     for i := 0; i < numWorkers; i++ {
//         go a.vectorizationWorker(ctx, i)
//     }
// }
//
// func (a *API) vectorizationWorker(ctx context.Context, id int) {
//     for {
//         select {
//         case <-ctx.Done():
//             return
//         default:
//             // Fetch next document from queue
//             // Vectorize it
//             // Update in store
//         }
//     }
// }

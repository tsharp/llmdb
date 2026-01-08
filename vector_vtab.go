package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"modernc.org/sqlite/vtab"
)

// VectorModule implements a virtual table for vector similarity search
type VectorModule struct {
	dimensions int
	metric     string // "cosine", "euclidean", or "dot"
	store      *DocumentStore
}

// VectorTable represents a vector search table instance
type VectorTable struct {
	module     *VectorModule
	dbId       string
	store      *DocumentStore
	dimensions int
	metric     string
}

// VectorCursor scans through search results
type VectorCursor struct {
	table       *VectorTable
	results     []SearchResult
	currentRow  int
	queryVector []float32
}

// RegisterVectorModule registers the vector search virtual table module
func RegisterVectorModule(db *sql.DB, store *DocumentStore) error {
	module := &VectorModule{
		dimensions: 768, // 384 - default
		metric:     "cosine",
		store:      store,
	}

	return vtab.RegisterModule(db, "vec_search", module)
}

// Create is called when CREATE VIRTUAL TABLE is executed
func (m *VectorModule) Create(ctx vtab.Context, args []string) (vtab.Table, error) {
	return m.Connect(ctx, args)
}

// Connect is called to connect to the virtual table
func (m *VectorModule) Connect(ctx vtab.Context, args []string) (vtab.Table, error) {
	// args[0] = module name, args[1] = database name, args[2] = table name
	// args[3+] = module arguments from USING vec_search(...)

	table := &VectorTable{
		module:     m,
		dimensions: m.dimensions,
		metric:     m.metric,
		store:      m.store,
	}

	// Parse module arguments if provided
	if len(args) > 3 {
		for _, arg := range args[3:] {
			parts := strings.SplitN(arg, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), "'\"")

			switch key {
			case "dim", "dimensions":
				if dim, err := strconv.Atoi(value); err == nil {
					table.dimensions = dim
				}
			case "metric":
				table.metric = value
			case "db", "database":
				table.dbId = value
			}
		}
	}

	// Declare the table schema
	schema := fmt.Sprintf(
		"CREATE TABLE x(doc_id TEXT, distance FLOAT, rank INT, db_id HIDDEN TEXT, query_vector HIDDEN BLOB)",
	)

	if err := ctx.Declare(schema); err != nil {
		return nil, err
	}

	return table, nil
}

// BestIndex helps SQLite's query planner choose the best way to scan this table
func (t *VectorTable) BestIndex(info *vtab.IndexInfo) error {
	// We need a query_vector constraint to perform vector search
	hasQueryVector := false
	argIdx := 0

	for i := range info.Constraints {
		constraint := &info.Constraints[i]

		if !constraint.Usable {
			continue
		}

		// Look for query_vector = ? constraint (column 4)
		if constraint.Column == 4 && constraint.Op == vtab.OpEQ {
			constraint.ArgIndex = argIdx
			constraint.Omit = true
			hasQueryVector = true
			argIdx++
		}

		// Look for db_id = ? constraint (column 3)
		if constraint.Column == 3 && constraint.Op == vtab.OpEQ {
			constraint.ArgIndex = argIdx
			constraint.Omit = true
			argIdx++
		}
	}

	if !hasQueryVector {
		// Very high cost if no query vector provided
		info.EstimatedCost = 1e10
		info.EstimatedRows = 0
		return nil
	}

	// Low cost for vector search
	info.EstimatedCost = 100
	info.EstimatedRows = 10
	info.IdxNum = 1 // Indicate we have a valid index

	return nil
}

// Open creates a new cursor for scanning
func (t *VectorTable) Open() (vtab.Cursor, error) {
	return &VectorCursor{
		table:      t,
		currentRow: -1,
	}, nil
}

// Disconnect is called when the table is no longer needed
func (t *VectorTable) Disconnect() error {
	return nil
}

// Destroy is called when DROP TABLE is executed
func (t *VectorTable) Destroy() error {
	return nil
}

// Filter initializes the cursor with search parameters
func (c *VectorCursor) Filter(idxNum int, idxStr string, vals []vtab.Value) error {
	if len(vals) == 0 {
		return fmt.Errorf("query vector required")
	}

	// Extract query vector from first argument
	queryVectorBytes, ok := vals[0].([]byte)
	if !ok {
		return fmt.Errorf("query vector must be a blob")
	}

	c.queryVector = deserializeVector(queryVectorBytes)

	// Extract db_id if provided
	dbId := c.table.dbId
	if len(vals) > 1 {
		if id, ok := vals[1].(string); ok {
			dbId = id
		}
	}

	if dbId == "" {
		return fmt.Errorf("database id required")
	}

	// TODO: Virtual table needs to be updated to work with dynamic table names
	// For now, this feature is disabled until the table name can be determined
	return fmt.Errorf("vector virtual table not yet compatible with dynamic tables")

	// Perform vector search
	// Note: This needs a table name parameter which isn't available in the current vtab design
	// results, err := c.table.store.SearchVector(dbId, tableName, c.queryVector, 100)
	// if err != nil {
	// 	return fmt.Errorf("vector search failed: %w", err)
	// }

	// c.results = results
	// c.currentRow = 0

	// return nil
}

// Column returns the value for the requested column
func (c *VectorCursor) Column(col int) (vtab.Value, error) {
	if c.currentRow < 0 || c.currentRow >= len(c.results) {
		return nil, fmt.Errorf("invalid row")
	}

	result := c.results[c.currentRow]

	switch col {
	case 0: // doc_id
		return result.Document.ID, nil
	case 1: // distance
		return result.Score, nil
	case 2: // rank
		return int64(result.Rank), nil
	case 3: // db_id (HIDDEN)
		return result.Document.DB, nil
	case 4: // query_vector (HIDDEN)
		return serializeVector(c.queryVector), nil
	default:
		return nil, fmt.Errorf("invalid column: %d", col)
	}
}

// Next advances to the next row
func (c *VectorCursor) Next() error {
	c.currentRow++
	return nil
}

// Eof returns true when we've scanned all rows
func (c *VectorCursor) Eof() bool {
	return c.currentRow >= len(c.results)
}

// Rowid returns the current rowid
func (c *VectorCursor) Rowid() (int64, error) {
	return int64(c.currentRow), nil
}

// Close closes the cursor
func (c *VectorCursor) Close() error {
	c.results = nil
	return nil
}

// Vector similarity functions

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func euclideanDistance(a, b []float32) float64 {
	if len(a) != len(b) {
		return math.MaxFloat64
	}

	var sum float64
	for i := range a {
		diff := float64(a[i]) - float64(b[i])
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

func dotProduct(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var product float64
	for i := range a {
		product += float64(a[i]) * float64(b[i])
	}

	return product
}

// Update SearchVector in store to use actual vector similarity
func (s *DocumentStore) searchVectorSimilarity(dbId, tableName string, queryVector []float32, limit int, metric string, filters map[string]interface{}) ([]SearchResult, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	// Build filter clause
	filterClause, filterArgs := buildFilterClause(filters, "")

	query := fmt.Sprintf(`
		SELECT id, content, metadata, tags, vector, created_at, updated_at, is_embedded
		FROM "%s"
		WHERE is_embedded = 1 AND vector IS NOT NULL%s
	`, tableName, filterClause)

	rows, err := db.Query(query, filterArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var doc Document
		var metadataJSON string
		var tagsStr string
		var vectorBytes []byte
		var isEmbedded int

		err := rows.Scan(&doc.ID, &doc.Content, &metadataJSON, &tagsStr, &vectorBytes,
			&doc.CreatedAt, &doc.UpdatedAt, &isEmbedded)
		if err != nil {
			continue
		}

		doc.DB = dbId
		doc.Table = tableName
		doc.IsEmbedded = isEmbedded == 1

		if metadataJSON != "" {
			_ = json.Unmarshal([]byte(metadataJSON), &doc.Metadata)
		}

		if tagsStr != "" {
			doc.Tags = strings.Split(tagsStr, ",")
		}

		if len(vectorBytes) > 0 {
			docVector := deserializeVector(vectorBytes)

			// Calculate similarity based on metric
			var score float64
			switch metric {
			case "cosine":
				score = cosineSimilarity(queryVector, docVector)
			case "euclidean":
				score = -euclideanDistance(queryVector, docVector) // Negative so higher is better
			case "dot":
				score = dotProduct(queryVector, docVector)
			default:
				score = cosineSimilarity(queryVector, docVector)
			}

			results = append(results, SearchResult{
				Document: doc,
				Score:    score,
			})
		}
	}

	// Sort by score (higher is better)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit results
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	// Add ranks
	for i := range results {
		results[i].Rank = i + 1
	}

	return results, rows.Err()
}

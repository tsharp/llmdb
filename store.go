package main

import (
	"database/sql"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// DocumentStore manages document storage across databases
// Each database gets its own SQLite database file
type DocumentStore struct {
	baseDir string
	dbs     map[string]*sql.DB // db name -> db connection
}

// NewDocumentStore creates a new document store
func NewDocumentStore(baseDir string) (*DocumentStore, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base directory: %w", err)
	}

	return &DocumentStore{
		baseDir: baseDir,
		dbs:     make(map[string]*sql.DB),
	}, nil
}

// getDB returns the database connection for a database, creating it if needed
func (s *DocumentStore) getDB(dbId string) (*sql.DB, error) {
	// Check if we already have a connection
	if db, exists := s.dbs[dbId]; exists {
		return db, nil
	}

	// Create new database file for this database
	dbPath := filepath.Join(s.baseDir, fmt.Sprintf("%s.db", dbId))
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Initialize schema
	if err := s.initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	s.dbs[dbId] = db
	return db, nil
}

// initSchema creates the necessary tables and indexes
func (s *DocumentStore) initSchema(db *sql.DB) error {
	// No default schema anymore - tables are created dynamically
	return nil
}

// ensureTable creates a table if it doesn't exist
func (s *DocumentStore) ensureTable(db *sql.DB, tableName string) error {
	// Sanitize table name (only allow alphanumeric and underscores)
	if !isValidTableName(tableName) {
		return fmt.Errorf("invalid table name: %s", tableName)
	}

	// Create main documents table for this collection
	docTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS "%s" (
			id TEXT PRIMARY KEY,
			content TEXT NOT NULL,
			metadata TEXT,
			tags TEXT,
			vector BLOB,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			is_embedded INTEGER DEFAULT 0
		);
	`, tableName)

	if _, err := db.Exec(docTableSQL); err != nil {
		return fmt.Errorf("failed to create table %s: %w", tableName, err)
	}

	// Create FTS5 virtual table
	ftsTableSQL := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS "%s_fts" USING fts5(
			id UNINDEXED,
			content,
			content='%s',
			content_rowid='rowid'
		);
	`, tableName, tableName)

	if _, err := db.Exec(ftsTableSQL); err != nil {
		return fmt.Errorf("failed to create FTS table for %s: %w", tableName, err)
	}

	// Create triggers to keep FTS table in sync
	triggerAI := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS "%s_ai" AFTER INSERT ON "%s" BEGIN
			INSERT INTO "%s_fts"(rowid, id, content)
			VALUES (new.rowid, new.id, new.content);
		END;
	`, tableName, tableName, tableName)

	triggerAD := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS "%s_ad" AFTER DELETE ON "%s" BEGIN
			DELETE FROM "%s_fts" WHERE rowid = old.rowid;
		END;
	`, tableName, tableName, tableName)

	triggerAU := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS "%s_au" AFTER UPDATE ON "%s" BEGIN
			UPDATE "%s_fts" SET content = new.content WHERE rowid = old.rowid;
		END;
	`, tableName, tableName, tableName)

	for _, trigger := range []string{triggerAI, triggerAD, triggerAU} {
		if _, err := db.Exec(trigger); err != nil {
			return fmt.Errorf("failed to create trigger for %s: %w", tableName, err)
		}
	}

	// Create indexes
	idxCreatedAt := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS "idx_%s_created_at" ON "%s"(created_at)`, tableName, tableName)
	idxUpdatedAt := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS "idx_%s_updated_at" ON "%s"(updated_at)`, tableName, tableName)
	idxEmbedded := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS "idx_%s_embedded" ON "%s"(is_embedded)`, tableName, tableName)
	idxTags := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS "idx_%s_tags" ON "%s"(tags)`, tableName, tableName)

	for _, idx := range []string{idxCreatedAt, idxUpdatedAt, idxEmbedded, idxTags} {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index for %s: %w", tableName, err)
		}
	}

	return nil
}

// isValidTableName checks if table name contains only safe characters
func isValidTableName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// StoreDocument stores a document in the specified database and table
func (s *DocumentStore) StoreDocument(dbId, tableName string, doc *Document) error {
	db, err := s.getDB(dbId)
	if err != nil {
		return err
	}

	// Ensure table exists
	if err := s.ensureTable(db, tableName); err != nil {
		return err
	}

	// Generate ID if not provided
	if doc.ID == "" {
		doc.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}
	doc.UpdatedAt = now
	doc.DB = dbId
	doc.Table = tableName

	// Serialize metadata
	metadataJSON, err := json.Marshal(doc.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Serialize tags as comma-separated string
	tagsStr := strings.Join(doc.Tags, ",")

	// Serialize vector if present
	var vectorBytes []byte
	if len(doc.Vector) > 0 {
		vectorBytes = serializeVector(doc.Vector)
		doc.IsEmbedded = true
	}

	query := fmt.Sprintf(`
		INSERT INTO "%s" (id, content, metadata, tags, vector, created_at, updated_at, is_embedded)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			metadata = excluded.metadata,
			tags = excluded.tags,
			vector = excluded.vector,
			updated_at = excluded.updated_at,
			is_embedded = excluded.is_embedded
	`, tableName)

	_, err = db.Exec(query, doc.ID, doc.Content, string(metadataJSON),
		tagsStr, vectorBytes, doc.CreatedAt, doc.UpdatedAt, boolToInt(doc.IsEmbedded))

	return err
}

// GetDocument retrieves a document by ID from the specified database and table
func (s *DocumentStore) GetDocument(dbId, tableName, id string) (*Document, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT id, content, metadata, tags, vector, created_at, updated_at, is_embedded
		FROM "%s"
		WHERE id = ?
	`, tableName)

	var doc Document
	var metadataJSON string
	var tagsStr string
	var vectorBytes []byte
	var isEmbedded int

	err = db.QueryRow(query, id).Scan(
		&doc.ID, &doc.Content, &metadataJSON, &tagsStr, &vectorBytes,
		&doc.CreatedAt, &doc.UpdatedAt, &isEmbedded,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document not found")
	}
	if err != nil {
		return nil, err
	}

	doc.DB = dbId
	doc.Table = tableName
	doc.IsEmbedded = isEmbedded == 1

	// Deserialize metadata
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &doc.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	// Deserialize tags
	if tagsStr != "" {
		doc.Tags = strings.Split(tagsStr, ",")
	}

	// Deserialize vector
	if len(vectorBytes) > 0 {
		doc.Vector = deserializeVector(vectorBytes)
	}

	return &doc, nil
}

// DeleteDocument deletes a document by ID from the specified database and table
func (s *DocumentStore) DeleteDocument(dbId, tableName, id string) error {
	db, err := s.getDB(dbId)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`DELETE FROM "%s" WHERE id = ?`, tableName)
	result, err := db.Exec(query, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("document not found")
	}

	return nil
}

// SearchFullText performs full-text search on documents
func (s *DocumentStore) SearchFullText(dbId, tableName, query string, limit int, filters map[string]interface{}) ([]SearchResult, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 10
	}

	// Build filter clause
	filterClause, filterArgs := buildFilterClause(filters, "d")

	// FTS5 query with ranking
	sqlQuery := fmt.Sprintf(`
		SELECT d.id, d.content, d.metadata, d.tags, d.vector, d.created_at, d.updated_at, 
		       d.is_embedded, bm25("%s_fts") as score
		FROM "%s_fts"
		JOIN "%s" d ON "%s_fts".rowid = d.rowid
		WHERE "%s_fts" MATCH ?%s
		ORDER BY score
		LIMIT ?
	`, tableName, tableName, tableName, tableName, tableName, filterClause)

	// Prepare arguments: query, filter args, limit
	queryArgs := []interface{}{query}
	queryArgs = append(queryArgs, filterArgs...)
	queryArgs = append(queryArgs, limit)

	rows, err := db.Query(sqlQuery, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("full-text search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var doc Document
		var metadataJSON string
		var tagsStr string
		var vectorBytes []byte
		var isEmbedded int
		var score float64

		err := rows.Scan(&doc.ID, &doc.Content, &metadataJSON, &tagsStr, &vectorBytes,
			&doc.CreatedAt, &doc.UpdatedAt, &isEmbedded, &score)
		if err != nil {
			return nil, err
		}

		doc.DB = dbId
		doc.Table = tableName
		doc.IsEmbedded = isEmbedded != 0

		if metadataJSON != "" {
			json.Unmarshal([]byte(metadataJSON), &doc.Metadata)
		}
		if tagsStr != "" {
			doc.Tags = strings.Split(tagsStr, ",")
		}
		if len(vectorBytes) > 0 {
			doc.Vector = deserializeVector(vectorBytes)
		}

		results = append(results, SearchResult{
			Document: doc,
			Score:    score,
		})
	}

	return results, rows.Err()
}

// SearchVector performs vector similarity search
func (s *DocumentStore) SearchVector(dbId, tableName string, queryVector []float32, limit int, filters map[string]interface{}) ([]SearchResult, error) {
	return s.searchVectorSimilarity(dbId, tableName, queryVector, limit, "cosine", filters)
}

// ListPartitions returns information about all databases (deprecated, use ListDatabases)
func (s *DocumentStore) ListPartitions() ([]DBInfo, error) {
	return s.ListDatabases()
}

// ListDatabases returns information about all databases
func (s *DocumentStore) ListDatabases() ([]DBInfo, error) {
	// Build a map of all database names (from both disk files and open connections)
	dbNames := make(map[string]bool)

	// Add databases from open connections
	for name := range s.dbs {
		dbNames[name] = true
	}

	// Add databases from disk files
	files, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read data directory %s: %w", s.baseDir, err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".db") {
			continue
		}
		dbName := strings.TrimSuffix(file.Name(), ".db")
		dbNames[dbName] = true
	}

	// Get info for all databases
	var databases []DBInfo
	for dbName := range dbNames {
		info, err := s.getDBInfo(dbName)
		if err != nil {
			// Log the error but don't fail completely
			fmt.Printf("Warning: failed to get info for database %s: %v\n", dbName, err)
			continue
		}
		databases = append(databases, info)
	}

	return databases, nil
}

// ListDocuments returns a list of documents in a database table with pagination
func (s *DocumentStore) ListDocuments(dbId, tableName string, limit, offset int) ([]Document, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT id, content, metadata, vector, created_at, updated_at, is_embedded
		FROM "%s"
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, tableName)

	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var documents []Document
	for rows.Next() {
		var doc Document
		var metadataJSON string
		var vectorBytes []byte
		var isEmbedded int

		err := rows.Scan(&doc.ID, &doc.Content, &metadataJSON, &vectorBytes,
			&doc.CreatedAt, &doc.UpdatedAt, &isEmbedded)
		if err != nil {
			return nil, err
		}

		doc.DB = dbId
		doc.Table = tableName
		doc.IsEmbedded = isEmbedded == 1

		if metadataJSON != "" {
			json.Unmarshal([]byte(metadataJSON), &doc.Metadata)
		}
		if len(vectorBytes) > 0 {
			doc.Vector = deserializeVector(vectorBytes)
		}

		documents = append(documents, doc)
	}

	return documents, rows.Err()
}

// DeleteDatabase deletes an entire database
func (s *DocumentStore) DeleteDatabase(dbId string) error {
	// Close connection if open
	if db, exists := s.dbs[dbId]; exists {
		db.Close()
		delete(s.dbs, dbId)
	}

	// Delete database file
	dbPath := filepath.Join(s.baseDir, fmt.Sprintf("%s.db", dbId))
	if err := os.Remove(dbPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("database not found")
		}
		return fmt.Errorf("failed to delete database file: %w", err)
	}

	return nil
}

// getDBInfo retrieves information about a specific database
func (s *DocumentStore) getDBInfo(dbId string) (DBInfo, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return DBInfo{}, err
	}

	var info DBInfo
	info.Name = dbId

	// Get document counts
	query := `
		SELECT 
			COUNT(*) as total,
			SUM(CASE WHEN is_vectorized = 1 THEN 1 ELSE 0 END) as vectorized,
			MIN(created_at) as first_created,
			MAX(updated_at) as last_updated
		FROM documents
	`

	var firstCreated, lastUpdated sql.NullString
	err = db.QueryRow(query).Scan(&info.DocumentCount, &info.EmbeddedCount, &firstCreated, &lastUpdated)
	if err != nil {
		return info, err
	}

	// Parse timestamps from strings
	if firstCreated.Valid && firstCreated.String != "" {
		if t, err := time.Parse(time.RFC3339, firstCreated.String); err == nil {
			info.CreatedAt = t
		}
	}
	if lastUpdated.Valid && lastUpdated.String != "" {
		if t, err := time.Parse(time.RFC3339, lastUpdated.String); err == nil {
			info.LastUpdated = t
		}
	}

	// Get file size
	dbPath := filepath.Join(s.baseDir, fmt.Sprintf("%s.db", dbId))
	if stat, err := os.Stat(dbPath); err == nil {
		info.SizeBytes = stat.Size()
	}

	return info, nil
}

// GetNonEmbeddedDocuments returns documents that need embedding from a database table
func (s *DocumentStore) GetNonEmbeddedDocuments(dbId, tableName string, limit int) ([]*Document, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT id, content, metadata, tags, vector, created_at, updated_at, is_embedded
		FROM "%s"
		WHERE is_embedded = 0
		ORDER BY created_at ASC
		LIMIT ?
	`, tableName)

	rows, err := db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query non-embedded documents: %w", err)
	}
	defer rows.Close()

	var documents []*Document
	for rows.Next() {
		var doc Document
		var metadataJSON string
		var tagsStr string
		var vectorBytes []byte
		var isEmbedded int

		err := rows.Scan(
			&doc.ID, &doc.Content, &metadataJSON, &tagsStr, &vectorBytes,
			&doc.CreatedAt, &doc.UpdatedAt, &isEmbedded,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan document: %w", err)
		}

		doc.DB = dbId
		doc.Table = tableName
		doc.IsEmbedded = isEmbedded != 0

		// Deserialize metadata
		if metadataJSON != "" {
			if err := json.Unmarshal([]byte(metadataJSON), &doc.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		// Deserialize tags
		if tagsStr != "" {
			doc.Tags = strings.Split(tagsStr, ",")
		}

		// Deserialize vector if present
		if len(vectorBytes) > 0 {
			doc.Vector = deserializeVector(vectorBytes)
		}

		documents = append(documents, &doc)
	}

	return documents, rows.Err()
}

// UpdateDocumentVector updates only the vector field of a document
func (s *DocumentStore) UpdateDocumentVector(dbId, tableName, docID string, vector []float32) error {
	db, err := s.getDB(dbId)
	if err != nil {
		return err
	}

	vectorBytes := serializeVector(vector)

	query := fmt.Sprintf(`
		UPDATE "%s"
		SET vector = ?, is_embedded = 1, updated_at = ?
		WHERE id = ?
	`, tableName)

	result, err := db.Exec(query, vectorBytes, time.Now(), docID)
	if err != nil {
		return fmt.Errorf("failed to update document vector: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("document not found")
	}

	return nil
}

// ListTables returns all table names in a database
func (s *DocumentStore) ListTables(dbId string) ([]string, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	query := `
		SELECT name FROM sqlite_master 
		WHERE type='table' 
		AND name NOT LIKE '%_fts%' 
		AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}

	return tables, rows.Err()
}

// Close closes all database connections
func (s *DocumentStore) Close() error {
	for _, db := range s.dbs {
		if err := db.Close(); err != nil {
			return err
		}
	}
	return nil
}

// Helper functions

// buildFilterClause builds a WHERE clause from filters
// Supports:
// - "tags": []string or string - filters by tags (AND logic for multiple tags)
// - "tag": string - filters by single tag
// - metadata fields: filters JSON metadata fields
// tableAlias: optional table alias prefix (e.g., "d" for "d.tags")
// Returns the WHERE clause SQL and the arguments for the query
func buildFilterClause(filters map[string]interface{}, tableAlias string) (string, []interface{}) {
	if len(filters) == 0 {
		return "", nil
	}

	var conditions []string
	var args []interface{}

	// Add table alias prefix if provided
	prefix := ""
	if tableAlias != "" {
		prefix = tableAlias + "."
	}

	for key, value := range filters {
		switch key {
		case "tags":
			// Handle tags filter - value can be string or []string
			var tagList []string
			switch v := value.(type) {
			case string:
				tagList = []string{v}
			case []interface{}:
				for _, t := range v {
					if str, ok := t.(string); ok {
						tagList = append(tagList, str)
					}
				}
			case []string:
				tagList = v
			}

			// For each tag, check if it's present in the comma-separated tags field
			for _, tag := range tagList {
				// Match: exact tag, or tag at start, or tag in middle/end
				conditions = append(conditions, fmt.Sprintf("(%stags = ? OR %stags LIKE ? OR %stags LIKE ? OR %stags LIKE ?)", prefix, prefix, prefix, prefix))
				args = append(args, tag, tag+",%", "%,"+tag+",%", "%,"+tag)
			}

		case "tag":
			// Single tag filter
			if tagStr, ok := value.(string); ok {
				conditions = append(conditions, fmt.Sprintf("(%stags = ? OR %stags LIKE ? OR %stags LIKE ? OR %stags LIKE ?)", prefix, prefix, prefix, prefix))
				args = append(args, tagStr, tagStr+",%", "%,"+tagStr+",%", "%,"+tagStr)
			}

		default:
			// Metadata field filter using JSON extraction
			// SQLite JSON syntax: json_extract(metadata, '$.field')
			conditions = append(conditions, fmt.Sprintf("json_extract(%smetadata, '$.%s') = ?", prefix, key))

			// SQLite json_extract returns values in their JSON types
			// For comparison, we need to use the actual value, not stringified
			switch v := value.(type) {
			case string:
				// For strings, json_extract returns them without quotes
				args = append(args, v)
			case int, int64, float64, float32:
				// For numbers, json_extract returns numeric values
				args = append(args, v)
			case bool:
				// For booleans, json_extract returns 0 or 1
				if v {
					args = append(args, 1)
				} else {
					args = append(args, 0)
				}
			default:
				// For other types, marshal to JSON and remove outer quotes if string
				jsonValue, _ := json.Marshal(value)
				valueStr := string(jsonValue)
				if len(valueStr) > 2 && valueStr[0] == '"' && valueStr[len(valueStr)-1] == '"' {
					valueStr = valueStr[1 : len(valueStr)-1]
				}
				args = append(args, valueStr)
			}
		}
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return " AND " + strings.Join(conditions, " AND "), args
}

func serializeVector(vector []float32) []byte {
	bytes := make([]byte, len(vector)*4)
	for i, v := range vector {
		binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(v))
	}
	return bytes
}

func deserializeVector(bytes []byte) []float32 {
	vector := make([]float32, len(bytes)/4)
	for i := range vector {
		bits := binary.LittleEndian.Uint32(bytes[i*4:])
		vector[i] = math.Float32frombits(bits)
	}
	return vector
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

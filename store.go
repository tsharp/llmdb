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

	for _, idx := range []string{idxCreatedAt, idxUpdatedAt, idxEmbedded} {
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

	// Serialize vector if present
	var vectorBytes []byte
	if len(doc.Vector) > 0 {
		vectorBytes = serializeVector(doc.Vector)
		doc.IsEmbedded = true
	}

	query := fmt.Sprintf(`
		INSERT INTO "%s" (id, content, metadata, vector, created_at, updated_at, is_embedded)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			content = excluded.content,
			metadata = excluded.metadata,
			vector = excluded.vector,
			updated_at = excluded.updated_at,
			is_embedded = excluded.is_embedded
	`, tableName)

	_, err = db.Exec(query, doc.ID, doc.Content, string(metadataJSON),
		vectorBytes, doc.CreatedAt, doc.UpdatedAt, boolToInt(doc.IsEmbedded))

	return err
}

// GetDocument retrieves a document by ID from the specified database and table
func (s *DocumentStore) GetDocument(dbId, tableName, id string) (*Document, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
		SELECT id, content, metadata, vector, created_at, updated_at, is_embedded
		FROM "%s"
		WHERE id = ?
	`, tableName)

	var doc Document
	var metadataJSON string
	var vectorBytes []byte
	var isEmbedded int

	err = db.QueryRow(query, id).Scan(
		&doc.ID, &doc.Content, &metadataJSON, &vectorBytes,
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
func (s *DocumentStore) SearchFullText(dbId, tableName, query string, limit int) ([]SearchResult, error) {
	db, err := s.getDB(dbId)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 10
	}

	// FTS5 query with ranking
	sqlQuery := fmt.Sprintf(`
		SELECT d.id, d.content, d.metadata, d.vector, d.created_at, d.updated_at, 
		       d.is_embedded, bm25(fts) as score
		FROM "%s_fts" fts
		JOIN "%s" d ON fts.rowid = d.rowid
		WHERE fts MATCH ?
		ORDER BY score
		LIMIT ?
	`, tableName, tableName)

	rows, err := db.Query(sqlQuery, query, limit)
	if err != nil {
		return nil, fmt.Errorf("full-text search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var doc Document
		var metadataJSON string
		var vectorBytes []byte
		var isEmbedded int
		var score float64

		err := rows.Scan(&doc.ID, &doc.Content, &metadataJSON, &vectorBytes,
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
func (s *DocumentStore) SearchVector(dbId, tableName string, queryVector []float32, limit int) ([]SearchResult, error) {
	return s.searchVectorSimilarity(dbId, tableName, queryVector, limit, "cosine")
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
		SELECT id, content, metadata, vector, created_at, updated_at, is_embedded
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
		var vectorBytes []byte
		var isEmbedded int

		err := rows.Scan(
			&doc.ID, &doc.Content, &metadataJSON, &vectorBytes,
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

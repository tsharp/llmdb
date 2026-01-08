package main

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestStore(t *testing.T) (*DocumentStore, string) {
	t.Helper()

	// Create temporary directory for test database
	tmpDir, err := os.MkdirTemp("", "llmdb-filter-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	store, err := NewDocumentStore(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create document store: %v", err)
	}

	return store, tmpDir
}

func cleanupTestStore(t *testing.T, store *DocumentStore, tmpDir string) {
	t.Helper()
	store.Close()
	os.RemoveAll(tmpDir)
}

func TestFilteringByTags(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer cleanupTestStore(t, store, tmpDir)

	dbName := "test_db"
	tableName := "documents"

	// Insert test documents
	docs := []*Document{
		{
			ID:      "doc1",
			Content: "Introduction to Python programming for beginners",
			Tags:    []string{"python", "programming", "beginner"},
			Metadata: map[string]interface{}{
				"author":     "Alice",
				"category":   "tutorial",
				"difficulty": "easy",
			},
		},
		{
			ID:      "doc2",
			Content: "Advanced JavaScript patterns and best practices",
			Tags:    []string{"javascript", "programming", "advanced"},
			Metadata: map[string]interface{}{
				"author":     "Bob",
				"category":   "tutorial",
				"difficulty": "hard",
			},
		},
		{
			ID:      "doc3",
			Content: "Machine Learning with Python",
			Tags:    []string{"python", "ml", "advanced"},
			Metadata: map[string]interface{}{
				"author":     "Charlie",
				"category":   "guide",
				"difficulty": "medium",
			},
		},
		{
			ID:      "doc4",
			Content: "Web development fundamentals using JavaScript",
			Tags:    []string{"javascript", "web", "beginner"},
			Metadata: map[string]interface{}{
				"author":     "Alice",
				"category":   "tutorial",
				"difficulty": "easy",
			},
		},
	}

	for _, doc := range docs {
		if err := store.StoreDocument(dbName, tableName, doc); err != nil {
			t.Fatalf("Failed to store document %s: %v", doc.ID, err)
		}
	}

	tests := []struct {
		name        string
		filters     map[string]interface{}
		wantIDs     []string
		description string
	}{
		{
			name:        "Filter by single tag - beginner",
			filters:     map[string]interface{}{"tag": "beginner"},
			wantIDs:     []string{"doc1", "doc4"},
			description: "Should return documents tagged as beginner",
		},
		{
			name:        "Filter by single tag - python",
			filters:     map[string]interface{}{"tag": "python"},
			wantIDs:     []string{"doc1", "doc3"},
			description: "Should return documents tagged as python",
		},
		{
			name:        "Filter by multiple tags - python AND advanced",
			filters:     map[string]interface{}{"tags": []string{"python", "advanced"}},
			wantIDs:     []string{"doc3"},
			description: "Should return documents with both python and advanced tags",
		},
		{
			name:        "Filter by multiple tags - javascript AND beginner",
			filters:     map[string]interface{}{"tags": []string{"javascript", "beginner"}},
			wantIDs:     []string{"doc4"},
			description: "Should return documents with both javascript and beginner tags",
		},
		{
			name:        "Filter by non-existent tag",
			filters:     map[string]interface{}{"tag": "nonexistent"},
			wantIDs:     []string{},
			description: "Should return no documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a search term that matches all documents or use wildcard
			results, err := store.SearchFullText(dbName, tableName, "JavaScript OR Python OR Web OR programming", 10, tt.filters)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			gotIDs := make([]string, len(results))
			for i, r := range results {
				gotIDs[i] = r.Document.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("%s: got %d results, want %d. Got IDs: %v, Want IDs: %v",
					tt.description, len(gotIDs), len(tt.wantIDs), gotIDs, tt.wantIDs)
				return
			}

			// Check if all expected IDs are present (order doesn't matter)
			wantMap := make(map[string]bool)
			for _, id := range tt.wantIDs {
				wantMap[id] = true
			}

			for _, id := range gotIDs {
				if !wantMap[id] {
					t.Errorf("%s: unexpected document ID: %s", tt.description, id)
				}
			}
		})
	}
}

func TestFilteringByMetadata(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer cleanupTestStore(t, store, tmpDir)

	dbName := "test_db"
	tableName := "documents"

	// Insert test documents
	docs := []*Document{
		{
			ID:      "doc1",
			Content: "Article by Alice from 2024",
			Tags:    []string{"article"},
			Metadata: map[string]interface{}{
				"author": "Alice",
				"year":   2024,
				"type":   "article",
			},
		},
		{
			ID:      "doc2",
			Content: "Article by Bob from 2024",
			Tags:    []string{"article"},
			Metadata: map[string]interface{}{
				"author": "Bob",
				"year":   2024,
				"type":   "article",
			},
		},
		{
			ID:      "doc3",
			Content: "Article by Alice from 2023",
			Tags:    []string{"article"},
			Metadata: map[string]interface{}{
				"author": "Alice",
				"year":   2023,
				"type":   "article",
			},
		},
		{
			ID:      "doc4",
			Content: "Book by Alice from 2024",
			Tags:    []string{"book"},
			Metadata: map[string]interface{}{
				"author": "Alice",
				"year":   2024,
				"type":   "book",
			},
		},
	}

	for _, doc := range docs {
		if err := store.StoreDocument(dbName, tableName, doc); err != nil {
			t.Fatalf("Failed to store document %s: %v", doc.ID, err)
		}
	}

	tests := []struct {
		name        string
		filters     map[string]interface{}
		wantIDs     []string
		description string
	}{
		{
			name:        "Filter by author - Alice",
			filters:     map[string]interface{}{"author": "Alice"},
			wantIDs:     []string{"doc1", "doc3", "doc4"},
			description: "Should return all documents by Alice",
		},
		{
			name:        "Filter by author - Bob",
			filters:     map[string]interface{}{"author": "Bob"},
			wantIDs:     []string{"doc2"},
			description: "Should return documents by Bob",
		},
		{
			name:        "Filter by year - 2024",
			filters:     map[string]interface{}{"year": 2024},
			wantIDs:     []string{"doc1", "doc2", "doc4"},
			description: "Should return documents from 2024",
		},
		{
			name:        "Filter by type - book",
			filters:     map[string]interface{}{"type": "book"},
			wantIDs:     []string{"doc4"},
			description: "Should return book type documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.SearchFullText(dbName, tableName, "Article OR Book", 10, tt.filters)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			gotIDs := make([]string, len(results))
			for i, r := range results {
				gotIDs[i] = r.Document.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("%s: got %d results, want %d. Got IDs: %v, Want IDs: %v",
					tt.description, len(gotIDs), len(tt.wantIDs), gotIDs, tt.wantIDs)
				return
			}

			// Check if all expected IDs are present
			wantMap := make(map[string]bool)
			for _, id := range tt.wantIDs {
				wantMap[id] = true
			}

			for _, id := range gotIDs {
				if !wantMap[id] {
					t.Errorf("%s: unexpected document ID: %s", tt.description, id)
				}
			}
		})
	}
}

func TestFilteringCombined(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer cleanupTestStore(t, store, tmpDir)

	dbName := "test_db"
	tableName := "documents"

	// Insert test documents
	docs := []*Document{
		{
			ID:      "doc1",
			Content: "Python tutorial for beginners",
			Tags:    []string{"python", "tutorial", "beginner"},
			Metadata: map[string]interface{}{
				"author":     "Alice",
				"difficulty": "easy",
				"year":       2024,
			},
		},
		{
			ID:      "doc2",
			Content: "Advanced Python tutorial",
			Tags:    []string{"python", "tutorial", "advanced"},
			Metadata: map[string]interface{}{
				"author":     "Bob",
				"difficulty": "hard",
				"year":       2024,
			},
		},
		{
			ID:      "doc3",
			Content: "Python guide for beginners",
			Tags:    []string{"python", "guide", "beginner"},
			Metadata: map[string]interface{}{
				"author":     "Alice",
				"difficulty": "easy",
				"year":       2023,
			},
		},
	}

	for _, doc := range docs {
		if err := store.StoreDocument(dbName, tableName, doc); err != nil {
			t.Fatalf("Failed to store document %s: %v", doc.ID, err)
		}
	}

	tests := []struct {
		name        string
		filters     map[string]interface{}
		wantIDs     []string
		description string
	}{
		{
			name: "Filter by tag and author",
			filters: map[string]interface{}{
				"tag":    "beginner",
				"author": "Alice",
			},
			wantIDs:     []string{"doc1", "doc3"},
			description: "Should return beginner documents by Alice",
		},
		{
			name: "Filter by tag and difficulty",
			filters: map[string]interface{}{
				"tag":        "tutorial",
				"difficulty": "easy",
			},
			wantIDs:     []string{"doc1"},
			description: "Should return easy tutorial documents",
		},
		{
			name: "Filter by multiple tags and year",
			filters: map[string]interface{}{
				"tags": []string{"python", "tutorial"},
				"year": 2024,
			},
			wantIDs:     []string{"doc1", "doc2"},
			description: "Should return python tutorials from 2024",
		},
		{
			name: "Filter with no matches",
			filters: map[string]interface{}{
				"tag":    "beginner",
				"author": "Bob",
			},
			wantIDs:     []string{},
			description: "Should return no documents (Bob has no beginner content)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.SearchFullText(dbName, tableName, "Python OR tutorial OR guide", 10, tt.filters)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			gotIDs := make([]string, len(results))
			for i, r := range results {
				gotIDs[i] = r.Document.ID
			}

			if len(gotIDs) != len(tt.wantIDs) {
				t.Errorf("%s: got %d results, want %d. Got IDs: %v, Want IDs: %v",
					tt.description, len(gotIDs), len(tt.wantIDs), gotIDs, tt.wantIDs)
				return
			}

			// Check if all expected IDs are present
			wantMap := make(map[string]bool)
			for _, id := range tt.wantIDs {
				wantMap[id] = true
			}

			for _, id := range gotIDs {
				if !wantMap[id] {
					t.Errorf("%s: unexpected document ID: %s", tt.description, id)
				}
			}
		})
	}
}

func TestTagsStorageAndRetrieval(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer cleanupTestStore(t, store, tmpDir)

	dbName := "test_db"
	tableName := "documents"

	testCases := []struct {
		name string
		tags []string
	}{
		{"Single tag", []string{"tag1"}},
		{"Multiple tags", []string{"tag1", "tag2", "tag3"}},
		{"Tags with special chars", []string{"tag-1", "tag_2", "tag.3"}},
		{"Empty tags", []string{}},
		{"Nil tags", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := &Document{
				ID:      "test-" + tc.name,
				Content: "Test document for " + tc.name,
				Tags:    tc.tags,
				Metadata: map[string]interface{}{
					"test": true,
				},
			}

			// Store document
			if err := store.StoreDocument(dbName, tableName, doc); err != nil {
				t.Fatalf("Failed to store document: %v", err)
			}

			// Retrieve document
			retrieved, err := store.GetDocument(dbName, tableName, doc.ID)
			if err != nil {
				t.Fatalf("Failed to retrieve document: %v", err)
			}

			// Verify tags
			if len(tc.tags) == 0 && len(retrieved.Tags) == 0 {
				// Both empty, OK
				return
			}

			if len(retrieved.Tags) != len(tc.tags) {
				t.Errorf("Tag count mismatch: got %d, want %d", len(retrieved.Tags), len(tc.tags))
				return
			}

			for i, tag := range tc.tags {
				if retrieved.Tags[i] != tag {
					t.Errorf("Tag mismatch at index %d: got %s, want %s", i, retrieved.Tags[i], tag)
				}
			}
		})
	}
}

func TestMetadataFiltering(t *testing.T) {
	store, tmpDir := setupTestStore(t)
	defer cleanupTestStore(t, store, tmpDir)

	dbName := "test_db"
	tableName := "documents"

	// Insert test documents with various metadata types
	docs := []*Document{
		{
			ID:      "doc1",
			Content: "Document with string metadata",
			Metadata: map[string]interface{}{
				"status": "published",
				"count":  42,
				"active": true,
			},
		},
		{
			ID:      "doc2",
			Content: "Document with different values",
			Metadata: map[string]interface{}{
				"status": "draft",
				"count":  10,
				"active": false,
			},
		},
	}

	for _, doc := range docs {
		if err := store.StoreDocument(dbName, tableName, doc); err != nil {
			t.Fatalf("Failed to store document %s: %v", doc.ID, err)
		}
	}

	tests := []struct {
		name    string
		filters map[string]interface{}
		wantIDs []string
	}{
		{
			name:    "Filter by string value",
			filters: map[string]interface{}{"status": "published"},
			wantIDs: []string{"doc1"},
		},
		{
			name:    "Filter by numeric value",
			filters: map[string]interface{}{"count": 42},
			wantIDs: []string{"doc1"},
		},
		{
			name:    "Filter by boolean true",
			filters: map[string]interface{}{"active": true},
			wantIDs: []string{"doc1"},
		},
		{
			name:    "Filter by boolean false",
			filters: map[string]interface{}{"active": false},
			wantIDs: []string{"doc2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := store.SearchFullText(dbName, tableName, "Document", 10, tt.filters)
			if err != nil {
				t.Fatalf("Search failed: %v", err)
			}

			if len(results) != len(tt.wantIDs) {
				t.Errorf("Got %d results, want %d", len(results), len(tt.wantIDs))
			}

			for i, result := range results {
				if result.Document.ID != tt.wantIDs[i] {
					t.Errorf("Result %d: got ID %s, want %s", i, result.Document.ID, tt.wantIDs[i])
				}
			}
		})
	}
}

func BenchmarkFilterByTags(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "llmdb-bench-*")
	defer os.RemoveAll(tmpDir)

	store, _ := NewDocumentStore(tmpDir)
	defer store.Close()

	dbName := "bench_db"
	tableName := "documents"

	// Insert 1000 test documents
	for i := 0; i < 1000; i++ {
		tags := []string{"tag1", "tag2"}
		if i%2 == 0 {
			tags = append(tags, "even")
		} else {
			tags = append(tags, "odd")
		}

		doc := &Document{
			ID:      filepath.Join("doc", string(rune(i))),
			Content: "Test document content for benchmarking",
			Tags:    tags,
			Metadata: map[string]interface{}{
				"index": i,
			},
		}
		store.StoreDocument(dbName, tableName, doc)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		filters := map[string]interface{}{"tag": "even"}
		_, err := store.SearchFullText(dbName, tableName, "content", 100, filters)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

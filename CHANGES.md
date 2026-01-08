# Filtering Implementation Summary

## Overview
Added comprehensive filtering capabilities to LLMDB, allowing documents to be filtered by tags and metadata fields during searches.

## Changes Made

### 1. Schema Updates ([schema.sql](schema.sql))
- Added `tags TEXT` column to documents table
- Added index on `tags` column for efficient filtering
- Tags are stored as comma-separated strings for fast querying

### 2. Data Models ([models.go](models.go))
- Added `Tags []string` field to `Document` struct
- Added `Tags []string` field to `StoreDocumentRequest` struct
- Both fields support JSON serialization/deserialization

### 3. Storage Layer ([store.go](store.go))
- Updated `ensureTable()` to create tables with `tags` column and index
- Updated `StoreDocument()` to serialize and store tags as comma-separated values
- Updated `GetDocument()` to deserialize tags from storage
- Updated `GetNonEmbeddedDocuments()` to include tags
- Added `buildFilterClause()` helper function that:
  - Handles tag filtering (single tag or multiple tags with AND logic)
  - Handles metadata field filtering using SQLite JSON functions
  - Supports combining multiple filters with AND logic
- Updated `SearchFullText()` to accept and apply filters
- Updated `SearchVector()` signature to accept filters

### 4. Vector Search ([vector_vtab.go](vector_vtab.go))
- Updated `searchVectorSimilarity()` to:
  - Accept filters parameter
  - Apply filters to vector search queries
  - Include tags in result documents

### 5. API Layer ([api.go](api.go))
- Updated `StoreDocument()` handler to accept tags from request
- Updated `SearchDocuments()` handler to pass filters to:
  - Full-text search
  - Vector search
- Filters are passed from the `SearchRequest.Filters` field

## Filter Capabilities

### Tag Filtering
```json
{
  "filters": {
    "tag": "tutorial"  // Single tag
  }
}
```
or
```json
{
  "filters": {
    "tags": ["python", "advanced"]  // Multiple tags (AND logic)
  }
}
```

### Metadata Filtering
```json
{
  "filters": {
    "author": "John Doe",
    "year": 2024,
    "category": "research"
  }
}
```

### Combined Filtering
```json
{
  "filters": {
    "tags": ["ml", "tutorial"],
    "author": "Jane Smith",
    "difficulty": "easy"
  }
}
```

## Filter Implementation Details

### Tags
- **Storage**: Comma-separated string (e.g., "tag1,tag2,tag3")
- **Indexing**: Standard SQLite index on the tags column
- **Matching**: Pattern matching using LIKE operators to find tags in the comma-separated list
- **Logic**: Multiple tags use AND logic (document must have ALL specified tags)

### Metadata
- **Storage**: JSON text field (unchanged from existing implementation)
- **Filtering**: Uses SQLite's `json_extract()` function to access nested fields
- **Matching**: Exact value matching after JSON extraction
- **Types**: Supports strings, numbers, booleans in metadata filters

### Performance Considerations
- Tag filtering is efficient due to column indexing
- Metadata filtering requires JSON extraction, which is slower than indexed columns
- Consider using tags for frequently filtered fields
- Use metadata for arbitrary/flexible fields that don't need optimized filtering

## Documentation
- Created [FILTERING.md](FILTERING.md) with comprehensive usage guide and examples
- Created [scripts/test-filtering.ps1](scripts/test-filtering.ps1) test script

## Backward Compatibility
- Existing databases automatically get the `tags` column when tables are accessed
- Existing documents will have NULL/empty tags until updated
- All existing API endpoints continue to work as before
- Tags and filters are optional - no breaking changes to existing code

## Testing
Run the test script to verify filtering functionality:
```powershell
.\scripts\test-filtering.ps1
```

The test script demonstrates:
1. Storing documents with tags and metadata
2. Filtering by single tag
3. Filtering by multiple tags
4. Filtering by metadata fields
5. Combining tag and metadata filters
6. Various filter combinations

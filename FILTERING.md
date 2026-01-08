# Document Filtering Guide

The LLMDB now supports filtering by tags and metadata fields in addition to full-text and vector search.

## Features

### 1. Tags
Documents can now have tags for easy categorization and filtering:
- Tags are stored as an array of strings
- Efficiently indexed for fast filtering
- Supports multiple tags per document

### 2. Metadata Filtering
Filter documents by any field in the JSON metadata object.

## API Usage

### Storing Documents with Tags

```bash
POST /db/{dbName}/{tableName}
```

```json
{
  "content": "Machine learning guide for beginners",
  "tags": ["ml", "tutorial", "beginner"],
  "metadata": {
    "author": "John Doe",
    "category": "education",
    "year": 2024
  }
}
```

### Searching with Filters

```bash
POST /db/{dbName}/{tableName}/search
```

#### Filter by Single Tag
```json
{
  "query": "machine learning",
  "type": "fulltext",
  "limit": 10,
  "filters": {
    "tag": "tutorial"
  }
}
```

#### Filter by Multiple Tags (AND logic)
```json
{
  "query": "deep learning",
  "type": "vector",
  "limit": 10,
  "filters": {
    "tags": ["ml", "advanced"]
  }
}
```

#### Filter by Metadata Fields
```json
{
  "query": "neural networks",
  "type": "hybrid",
  "limit": 10,
  "filters": {
    "author": "John Doe",
    "year": 2024,
    "category": "education"
  }
}
```

#### Combine Tags and Metadata Filters
```json
{
  "query": "transformers",
  "type": "fulltext",
  "limit": 10,
  "filters": {
    "tags": ["ml", "nlp"],
    "author": "Jane Smith",
    "category": "research"
  }
}
```

## Examples

### Example 1: Store documents with different tags

```bash
# Store a beginner tutorial
curl -X POST http://localhost:8080/db/knowledge/articles \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Introduction to Machine Learning",
    "tags": ["ml", "tutorial", "beginner"],
    "metadata": {"author": "Alice", "difficulty": "easy"}
  }'

# Store an advanced guide
curl -X POST http://localhost:8080/db/knowledge/articles \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Advanced Deep Learning Techniques",
    "tags": ["ml", "deep-learning", "advanced"],
    "metadata": {"author": "Bob", "difficulty": "hard"}
  }'

# Store a research paper
curl -X POST http://localhost:8080/db/knowledge/articles \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Transformer Architecture Research",
    "tags": ["ml", "nlp", "research"],
    "metadata": {"author": "Charlie", "year": 2024}
  }'
```

### Example 2: Search with tag filtering

```bash
# Find only beginner tutorials
curl -X POST http://localhost:8080/db/knowledge/articles/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "machine learning",
    "type": "fulltext",
    "filters": {"tag": "beginner"}
  }'

# Find ML research papers
curl -X POST http://localhost:8080/db/knowledge/articles/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "neural networks",
    "type": "vector",
    "filters": {"tags": ["ml", "research"]}
  }'
```

### Example 3: Search with metadata filtering

```bash
# Find articles by specific author
curl -X POST http://localhost:8080/db/knowledge/articles/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "learning",
    "type": "fulltext",
    "filters": {"author": "Alice"}
  }'

# Find hard difficulty articles from 2024
curl -X POST http://localhost:8080/db/knowledge/articles/search \
  -H "Content-Type: application/json" \
  -d '{
    "query": "techniques",
    "type": "fulltext",
    "filters": {
      "difficulty": "hard",
      "year": 2024
    }
  }'
```

## Filter Behavior

### Tag Matching
- Tag filters use exact string matching
- Multiple tags in a filter use AND logic (document must have ALL tags)
- Tags are case-sensitive

### Metadata Matching
- Metadata filters use exact value matching
- String values are case-sensitive
- Numeric values match exactly
- Boolean values (true/false) are supported
- Multiple metadata filters use AND logic

### Combining Filters
When combining tag and metadata filters, ALL conditions must be met (AND logic).

## Implementation Details

- **Tags Storage**: Stored as comma-separated values with an index for fast filtering
- **Metadata Storage**: Stored as JSON, filtered using SQLite's `json_extract()` function
- **Performance**: Tag filtering is optimized with indexes; metadata filtering uses JSON extraction

## Migration Notes

Existing databases will automatically add the `tags` column when tables are accessed. Existing documents will have `NULL` or empty tags until updated.

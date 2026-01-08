-- Main documents table
CREATE TABLE IF NOT EXISTS documents (
	id TEXT PRIMARY KEY,
	content TEXT NOT NULL,
	metadata TEXT, -- JSON
	tags TEXT, -- Comma-separated tags for filtering
	vector BLOB, -- Serialized float32 array
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL,
	is_vectorized INTEGER DEFAULT 0
);

-- FTS5 virtual table for full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
	id UNINDEXED,
	content,
	content='documents',
	content_rowid='rowid'
);

-- Triggers to keep FTS table in sync
CREATE TRIGGER IF NOT EXISTS documents_ai AFTER INSERT ON documents BEGIN
	INSERT INTO documents_fts(rowid, id, content)
	VALUES (new.rowid, new.id, new.content);
END;

CREATE TRIGGER IF NOT EXISTS documents_ad AFTER DELETE ON documents BEGIN
	DELETE FROM documents_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS documents_au AFTER UPDATE ON documents BEGIN
	UPDATE documents_fts SET content = new.content WHERE rowid = old.rowid;
END;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_documents_created_at ON documents(created_at);
CREATE INDEX IF NOT EXISTS idx_documents_updated_at ON documents(updated_at);
CREATE INDEX IF NOT EXISTS idx_documents_embedded ON documents(is_embedded);
CREATE INDEX IF NOT EXISTS idx_documents_tags ON documents(tags);

-- TODO: Add sqlite-vec extension initialization here
-- SELECT load_extension('vec0');
-- CREATE VIRTUAL TABLE IF NOT EXISTS vec_documents USING vec0(
--     id TEXT PRIMARY KEY,
--     embedding FLOAT[768]  -- Assuming 768-dimensional vectors https://huggingface.co/google/embeddinggemma-300m
-- );

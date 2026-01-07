# llmdb

A lightweight, embeddable vector database API built with Go and SQLite. llmdb provides a simple HTTP API for storing documents with vector embeddings and performing semantic similarity searches, making it ideal for local development, testing, or distributed environments.

## Overview

llmdb combines SQLite's reliability with custom vector search capabilities to create a minimal yet powerful document store. The project features:

- **Vector Search**: Store and query documents using high-dimensional embeddings (configurable dimensions)
- **RESTful API**: Simple HTTP endpoints for creating, updating, deleting, and searching documents
- **Flexible Embedding**: Support for external embedding services or stub implementations for testing
- **Dynamic Tables**: Create and manage multiple document collections with custom schemas
- **Sharding Support**: Designed to work in both single-instance and distributed configurations
- **Feature Flags**: Client-driven feature toggles for progressive functionality enhancement

---

## Disclaimer

*This project was generated with the assistance of Claude Sonnet 4.5 via GitHub Copilot in VS Code.*

*I include these tools for transparency and provenance tracking in case this is ever useful to others in the future.*

| Tool                | Version | Date       |
| ------------------- | ------- | ---------- |
| VsCode              | 1.107.1 | 2026.01.07 |
| github.copilot-chat | 0.35.3  | 2026.01.07 |
| Claude Sonnet       | 4.5     | 2026.01.07 |
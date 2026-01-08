package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/mux"
)

// Version is set via ldflags during build
var Version = "dev"

// Config holds application configuration
type Config struct {
	EmbeddingURL        string          `json:"embedding_url"`
	EmbeddingDimensions int             `json:"embedding_dimensions"`
	DataDir             string          `json:"data_dir"`
	Port                string          `json:"port"`
	InsecureSkipVerify  bool            `json:"insecure_skip_verify"` // Skip TLS certificate verification
	CACertPath          string          `json:"ca_cert_path"`         // Path to custom CA certificate
	Features            map[string]bool `json:"features"`             // Enabled features (true/false)
}

// loadConfig loads configuration from file with environment variable overrides
func loadConfig(configPath string) (*Config, error) {
	// Default config
	config := &Config{
		EmbeddingURL:        "stub",
		EmbeddingDimensions: 2560,
		DataDir:             "./data",
		Port:                "8080",
	}

	// Try to load from file
	if data, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
		log.Printf("Loaded configuration from %s", configPath)
	} else {
		log.Printf("Config file not found, using defaults")
	}

	// Environment variables override file config
	if url := os.Getenv("EMBEDDING_URL"); url != "" {
		config.EmbeddingURL = url
	}
	if dim := os.Getenv("EMBEDDING_DIMENSIONS"); dim != "" {
		if d, err := strconv.Atoi(dim); err == nil {
			config.EmbeddingDimensions = d
		}
	}
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		config.DataDir = dir
	}
	if port := os.Getenv("PORT"); port != "" {
		config.Port = port
	}
	if skip := os.Getenv("INSECURE_SKIP_VERIFY"); skip == "true" {
		config.InsecureSkipVerify = true
	}
	if cert := os.Getenv("CA_CERT_PATH"); cert != "" {
		config.CACertPath = cert
	}

	return config, nil
}

// main is the entry point for the Context Pipeline API service
func main() {
	// Print version information
	log.Printf("LLMDB version %s", Version)

	// Load configuration
	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize document store
	store, err := NewDocumentStore(config.DataDir)
	if err != nil {
		log.Fatalf("Failed to create document store: %v", err)
	}
	defer store.Close()

	// Initialize embedder
	var embedder Embedder
	if config.EmbeddingURL != "" && config.EmbeddingURL != "stub" {
		embedder, err = NewLlamaCppEmbedder(config.EmbeddingURL, config.EmbeddingDimensions, config.InsecureSkipVerify, config.CACertPath)
		if err != nil {
			log.Fatalf("Failed to create embedder: %v", err)
		}
		log.Printf("Using llama.cpp embedder at %s (dimension: %d)", config.EmbeddingURL, config.EmbeddingDimensions)
		if config.InsecureSkipVerify {
			log.Printf("WARNING: TLS certificate verification is disabled")
		}
	} else {
		embedder = NewStubEmbedder()
		log.Printf("Using stub embedder (no actual embedding)")
	}

	// Create API
	api := NewAPI(store, embedder, config)

	// Start background embedding worker
	stopWorker := make(chan struct{})
	if enabled, ok := config.Features["embedding_job"]; ok && enabled {
		go startEmbeddingWorker(store, embedder, stopWorker)
	} else {
		log.Println("Background embedding worker is disabled by configuration")
	}

	// Setup router
	r := mux.NewRouter()

	// Routes
	r.HandleFunc("/health", api.Health).Methods("GET")
	r.HandleFunc("/db", api.ListDatabases).Methods("GET")
	r.HandleFunc("/db/{dbName}", api.ListTables).Methods("GET")
	r.HandleFunc("/db/{dbName}", api.DeleteDatabase).Methods("DELETE")
	r.HandleFunc("/db/{dbName}/{tableName}", api.ListDocuments).Methods("GET")
	r.HandleFunc("/db/{dbName}/{tableName}", api.StoreDocument).Methods("POST")
	r.HandleFunc("/db/{dbName}/{tableName}/search", api.SearchDocuments).Methods("POST")
	r.HandleFunc("/db/{dbName}/{tableName}/{docId}", api.GetDocument).Methods("GET")
	r.HandleFunc("/db/{dbName}/{tableName}/{docId}", api.DeleteDocument).Methods("DELETE")

	// Middleware
	r.Use(loggingMiddleware)
	r.Use(corsMiddleware)

	// Start server
	addr := fmt.Sprintf(":%s", config.Port)
	fmt.Printf("Context Pipeline API starting on %s\n", addr)
	fmt.Printf("Data directory: %s\n", config.DataDir)
	fmt.Printf("Embedding service: %s\n", config.EmbeddingURL)
	fmt.Printf("Embedding dimensions: %d\n", config.EmbeddingDimensions)
	fmt.Printf("\nAvailable endpoints:\n")
	fmt.Printf("  GET    /health\n")
	fmt.Printf("  GET    /db\n")
	fmt.Printf("  GET    /db/{dbName}\n")
	fmt.Printf("  DELETE /db/{dbName}\n")
	fmt.Printf("  GET    /db/{dbName}/{tableName}\n")
	fmt.Printf("  POST   /db/{dbName}/{tableName}\n")
	fmt.Printf("  POST   /db/{dbName}/{tableName}/search\n")
	fmt.Printf("  GET    /db/{dbName}/{tableName}/{docId}\n")
	fmt.Printf("  DELETE /db/{dbName}/{tableName}/{docId}\n")
	fmt.Printf("\nUse X-Client-Features: embed=sync header to trigger immediate embedding\n")

	// Graceful shutdown
	go func() {
		if err := http.ListenAndServe(addr, r); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down gracefully...")
	close(stopWorker) // Stop the background worker
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Client-Features")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helpers

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// startEmbeddingWorker polls for non-embedded documents and processes them
func startEmbeddingWorker(store *DocumentStore, embedder Embedder, stop chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	log.Println("Background embedding worker started")

	for {
		select {
		case <-stop:
			log.Println("Background embedding worker stopped")
			return
		case <-ticker.C:
			processNonEmbeddedDocuments(store, embedder)
		}
	}
}

// processNonEmbeddedDocuments finds and embeds documents across all databases and tables
func processNonEmbeddedDocuments(store *DocumentStore, embedder Embedder) {
	log.Println("Embedding worker: checking for non-embedded documents...")

	// Get all databases
	databases, err := store.ListDatabases()
	if err != nil {
		log.Printf("Error listing databases for embedding: %v", err)
		return
	}

	log.Printf("Embedding worker: found %d databases to check", len(databases))
	totalProcessed := 0
	maxDocuments := 15 // Process up to 15 documents total per cycle

	for _, db := range databases {
		if totalProcessed >= maxDocuments {
			break
		}

		// Get all tables in this database
		tables, err := store.ListTables(db.Name)
		if err != nil {
			log.Printf("Error listing tables in database %s: %v", db.Name, err)
			continue
		}

		log.Printf("Embedding worker: found %d tables in database '%s'", len(tables), db.Name)

		// Process each table
		for _, tableName := range tables {
			if totalProcessed >= maxDocuments {
				break
			}

			// Calculate remaining capacity
			remaining := maxDocuments - totalProcessed

			log.Printf("Embedding worker: checking table '%s.%s' for up to %d documents", db.Name, tableName, remaining)

			// Get non-embedded documents from this table
			docs, err := store.GetNonEmbeddedDocuments(db.Name, tableName, remaining)
			if err != nil {
				log.Printf("Error getting non-embedded documents from %s.%s: %v", db.Name, tableName, err)
				continue
			}

			log.Printf("Embedding worker: found %d non-embedded documents in table '%s.%s'", len(docs), db.Name, tableName)

			if len(docs) == 0 {
				continue
			}

			log.Printf("Processing %d non-embedded documents from table '%s.%s'", len(docs), db.Name, tableName)

			for _, doc := range docs {
				// Create context with timeout for each document
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

				vector, err := embedder.Embed(ctx, doc.Content)
				cancel() // Clean up context immediately

				if err != nil {
					log.Printf("Failed to embed document %s in table %s.%s: %v", doc.ID, db.Name, tableName, err)
					continue
				}

				// Update the document with the vector
				if err := store.UpdateDocumentVector(db.Name, tableName, doc.ID, vector); err != nil {
					log.Printf("Failed to update document %s vector in table %s.%s: %v", doc.ID, db.Name, tableName, err)
					continue
				}

				totalProcessed++
			}
		}
	}

	if totalProcessed > 0 {
		log.Printf("Background embedding: processed %d documents", totalProcessed)
	} else {
		log.Println("Embedding worker: no documents to process")
	}
}

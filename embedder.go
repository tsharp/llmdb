package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// Embedder is the interface for converting text to vectors
type Embedder interface {
	// Embed converts text to a vector embedding
	Embed(ctx context.Context, text string) ([]float32, error)

	// EmbedBatch converts multiple texts to vector embeddings
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)

	// Dimensions returns the dimensionality of the embeddings
	Dimensions() int
}

// LlamaCppEmbedder calls llama.cpp server for embeddings
type LlamaCppEmbedder struct {
	baseURL    string
	dimensions int
	client     *http.Client
}

// NewLlamaCppEmbedder creates an embedder that calls llama.cpp
func NewLlamaCppEmbedder(baseURL string, dimensions int, insecureSkipVerify bool, caCertPath string) (*LlamaCppEmbedder, error) {
	// Create custom TLS config
	tlsConfig := &tls.Config{
		InsecureSkipVerify: insecureSkipVerify,
	}

	// Load custom CA cert if provided
	if caCertPath != "" {
		caCert, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
		tlsConfig.InsecureSkipVerify = false // Use proper verification with custom CA
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &LlamaCppEmbedder{
		baseURL:    baseURL,
		dimensions: dimensions,
		client: &http.Client{
			Transport: transport,
		},
	}, nil
}

// EmbeddingRequest for llama.cpp
type EmbeddingRequest struct {
	Content string `json:"content"`
}

func (e *LlamaCppEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := EmbeddingRequest{
		Content: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/embedding", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call embedding service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read raw response to determine format
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Try parsing as array first (simple float array)
	var embeddingArray []float32
	if err := json.Unmarshal(body, &embeddingArray); err == nil && len(embeddingArray) > 0 {
		if len(embeddingArray) != e.dimensions {
			return nil, fmt.Errorf("expected embedding dimension %d, got %d", e.dimensions, len(embeddingArray))
		}
		return embeddingArray, nil
	}

	// Try parsing as object with "embedding" field
	var embeddingResp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(body, &embeddingResp); err == nil && len(embeddingResp.Embedding) > 0 {
		if len(embeddingResp.Embedding) != e.dimensions {
			return nil, fmt.Errorf("expected embedding dimension %d, got %d", e.dimensions, len(embeddingResp.Embedding))
		}
		return embeddingResp.Embedding, nil
	}

	// Try parsing as array of objects with nested embedding (llama.cpp batch format)
	var batchResp []struct {
		Index     int         `json:"index"`
		Embedding [][]float32 `json:"embedding"`
	}
	if err := json.Unmarshal(body, &batchResp); err == nil && len(batchResp) > 0 {
		if len(batchResp[0].Embedding) > 0 && len(batchResp[0].Embedding[0]) > 0 {
			embedding := batchResp[0].Embedding[0]
			if len(embedding) != e.dimensions {
				return nil, fmt.Errorf("expected embedding dimension %d, got %d", e.dimensions, len(embedding))
			}
			return embedding, nil
		}
	}

	// If all failed, return error with response preview
	preview := string(body)
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	return nil, fmt.Errorf("failed to parse embedding response. Preview: %s", preview)
}

func (e *LlamaCppEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vector, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		vectors[i] = vector
	}

	return vectors, nil
}

func (e *LlamaCppEmbedder) Dimensions() int {
	return e.dimensions
}

// StubEmbedder is a placeholder implementation for the Embedder interface
// Replace this with actual embedding service calls (Ollama, OpenAI, etc.)
type StubEmbedder struct {
	dimensions int
}

func NewStubEmbedder() *StubEmbedder {
	return &StubEmbedder{
		dimensions: 2560, // Qwen3 8b embedding size
	}
}

func (e *StubEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// TODO: Implement actual embedding logic
	// Example implementations:
	// 1. Call Ollama API: POST http://localhost:11434/api/embeddings
	// 2. Call OpenAI API: POST https://api.openai.com/v1/embeddings
	// 3. Call custom embedding service via gRPC

	// For now, return a stub vector
	vector := make([]float32, e.dimensions)
	for i := range vector {
		vector[i] = 0.1 // Placeholder values
	}

	return vector, nil
}

func (e *StubEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// TODO: Implement batch embedding for efficiency
	// Most embedding services support batch requests

	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		vector, err := e.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		vectors[i] = vector
	}

	return vectors, nil
}

func (e *StubEmbedder) Dimensions() int {
	return e.dimensions
}

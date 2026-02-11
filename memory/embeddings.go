package memory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIConfig configures the OpenAI embedding provider
type OpenAIConfig struct {
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	MaxRetries int
}

// DefaultOpenAIConfig returns default OpenAI configuration
func DefaultOpenAIConfig(apiKey string) OpenAIConfig {
	return OpenAIConfig{
		APIKey:     apiKey,
		BaseURL:    "https://api.openai.com/v1",
		Model:      "text-embedding-3-small",
		Timeout:    30 * time.Second,
		MaxRetries: 3,
	}
}

// openAIEmbeddingRequest is the request body for OpenAI embeddings API
type openAIEmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

// openAIEmbeddingResponse is the response from OpenAI embeddings API
type openAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// openAIErrorResponse represents an error from OpenAI API
type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// OpenAIProvider implements EmbeddingProvider using OpenAI's API
type OpenAIProvider struct {
	config     OpenAIConfig
	httpClient *http.Client
	dimension  int
}

// NewOpenAIProvider creates a new OpenAI embedding provider
func NewOpenAIProvider(config OpenAIConfig) (*OpenAIProvider, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	// Determine dimension based on model
	dimension := 1536 // default for text-embedding-3-small
	switch config.Model {
	case "text-embedding-3-large":
		dimension = 3072
	case "text-embedding-ada-002":
		dimension = 1536
	}

	return &OpenAIProvider{
		config:     config,
		httpClient: &http.Client{Timeout: config.Timeout},
		dimension:  dimension,
	}, nil
}

// Embed generates a single embedding
func (p *OpenAIProvider) Embed(text string) ([]float32, error) {
	embeddings, err := p.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates multiple embeddings in one call
func (p *OpenAIProvider) EmbedBatch(texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, fmt.Errorf("no texts provided")
	}
	if len(texts) > p.MaxBatchSize() {
		return nil, fmt.Errorf("batch size %d exceeds maximum %d", len(texts), p.MaxBatchSize())
	}

	reqBody := openAIEmbeddingRequest{
		Input: texts,
		Model: p.config.Model,
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", p.config.BaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(reqJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.config.APIKey))

	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		resp, err := p.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			var errResp openAIErrorResponse
			if err := json.Unmarshal(body, &errResp); err == nil {
				lastErr = fmt.Errorf("API error: %s", errResp.Error.Message)
			} else {
				lastErr = fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
			}

			// Retry on rate limit or server errors
			if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				continue
			}
			return nil, lastErr
		}

		var respBody openAIEmbeddingResponse
		if err := json.Unmarshal(body, &respBody); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		// Extract embeddings in order
		result := make([][]float32, len(texts))
		for _, item := range respBody.Data {
			if item.Index >= 0 && item.Index < len(result) {
				result[item.Index] = item.Embedding
			}
		}

		// Verify all embeddings were returned
		for i, emb := range result {
			if emb == nil {
				return nil, fmt.Errorf("missing embedding at index %d", i)
			}
		}

		return result, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Dimension returns the dimension of embeddings
func (p *OpenAIProvider) Dimension() int {
	return p.dimension
}

// MaxBatchSize returns the maximum batch size
func (p *OpenAIProvider) MaxBatchSize() int {
	// OpenAI supports up to 2048 texts per batch for embeddings
	return 2048
}

// GeminiProvider implements EmbeddingProvider using Google's Gemini API
// This is a placeholder for future implementation
type GeminiProvider struct {
	apiKey    string
	model     string
	dimension int
}

// NewGeminiProvider creates a new Gemini embedding provider
func NewGeminiProvider(apiKey, model string) (*GeminiProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}
	if model == "" {
		model = "embedding-001"
	}
	return &GeminiProvider{
		apiKey:    apiKey,
		model:     model,
		dimension: 768, // Gemini embedding dimension
	}, nil
}

// Embed generates a single embedding
func (p *GeminiProvider) Embed(text string) ([]float32, error) {
	// TODO: Implement Gemini embedding API call
	return nil, fmt.Errorf("Gemini provider not yet implemented")
}

// EmbedBatch generates multiple embeddings in one call
func (p *GeminiProvider) EmbedBatch(texts []string) ([][]float32, error) {
	// TODO: Implement Gemini batch embedding API call
	return nil, fmt.Errorf("Gemini provider not yet implemented")
}

// Dimension returns the dimension of embeddings
func (p *GeminiProvider) Dimension() int {
	return p.dimension
}

// MaxBatchSize returns the maximum batch size
func (p *GeminiProvider) MaxBatchSize() int {
	return 100 // Gemini typically supports smaller batches
}

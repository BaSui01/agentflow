package embedding

import (
	"context"
	"encoding/json"
	"time"
)

// OpenAIProvider implements embedding using OpenAI's API.
type OpenAIProvider struct {
	*BaseProvider
	cfg OpenAIConfig
}

// NewOpenAIProvider creates a new OpenAI embedding provider.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "text-embedding-3-large"
	}
	if cfg.Dimensions == 0 {
		cfg.Dimensions = 3072
	}

	return &OpenAIProvider{
		BaseProvider: NewBaseProvider(BaseConfig{
			Name:       "openai-embedding",
			BaseURL:    cfg.BaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.Model,
			Dimensions: cfg.Dimensions,
			MaxBatch:   2048,
			Timeout:    cfg.Timeout,
		}),
		cfg: cfg,
	}
}

type openAIEmbedRequest struct {
	Input          interface{} `json:"input"`
	Model          string      `json:"model"`
	Dimensions     int         `json:"dimensions,omitempty"`
	EncodingFormat string      `json:"encoding_format,omitempty"`
}

type openAIEmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed generates embeddings for the given inputs.
func (p *OpenAIProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	model := ChooseModel(req.Model, p.cfg.Model, "text-embedding-3-large")
	dims := req.Dimensions
	if dims == 0 {
		dims = p.cfg.Dimensions
	}

	body := openAIEmbedRequest{
		Input:      req.Input,
		Model:      model,
		Dimensions: dims,
	}
	if req.EncodingFormat != "" {
		body.EncodingFormat = req.EncodingFormat
	}

	respBody, err := p.DoRequest(ctx, "POST", "/v1/embeddings", body, map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}

	var oaResp openAIEmbedResponse
	if err := json.Unmarshal(respBody, &oaResp); err != nil {
		return nil, err
	}

	embeddings := make([]EmbeddingData, len(oaResp.Data))
	for i, d := range oaResp.Data {
		embeddings[i] = EmbeddingData{
			Index:     d.Index,
			Embedding: d.Embedding,
			Object:    d.Object,
		}
	}

	return &EmbeddingResponse{
		Provider:   p.Name(),
		Model:      oaResp.Model,
		Embeddings: embeddings,
		Usage: EmbeddingUsage{
			PromptTokens: oaResp.Usage.PromptTokens,
			TotalTokens:  oaResp.Usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// EmbedQuery embeds a single query.
func (p *OpenAIProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return p.BaseProvider.EmbedQuery(ctx, query, p.Embed)
}

// EmbedDocuments embeds multiple documents.
func (p *OpenAIProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return p.BaseProvider.EmbedDocuments(ctx, documents, p.Embed)
}

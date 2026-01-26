package embedding

import (
	"context"
	"encoding/json"
	"time"
)

// VoyageProvider implements embedding using Voyage AI's API.
type VoyageProvider struct {
	*BaseProvider
	cfg VoyageConfig
}

// NewVoyageProvider creates a new Voyage AI embedding provider.
func NewVoyageProvider(cfg VoyageConfig) *VoyageProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.voyageai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "voyage-3-large"
	}

	return &VoyageProvider{
		BaseProvider: NewBaseProvider(BaseConfig{
			Name:       "voyage-embedding",
			BaseURL:    cfg.BaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.Model,
			Dimensions: 1024, // voyage-3-large default
			MaxBatch:   128,
			Timeout:    cfg.Timeout,
		}),
		cfg: cfg,
	}
}

type voyageEmbedRequest struct {
	Input     []string `json:"input"`
	Model     string   `json:"model"`
	InputType string   `json:"input_type,omitempty"` // query, document
	Truncate  bool     `json:"truncation,omitempty"`
}

type voyageEmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Embed generates embeddings using Voyage AI.
func (p *VoyageProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	model := ChooseModel(req.Model, p.cfg.Model, "voyage-3-large")

	body := voyageEmbedRequest{
		Input:    req.Input,
		Model:    model,
		Truncate: req.Truncate,
	}

	// Map input type
	switch req.InputType {
	case InputTypeQuery, InputTypeCodeQuery:
		body.InputType = "query"
	case InputTypeDocument, InputTypeCodeDoc:
		body.InputType = "document"
	}

	respBody, err := p.DoRequest(ctx, "POST", "/v1/embeddings", body, map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}

	var vResp voyageEmbedResponse
	if err := json.Unmarshal(respBody, &vResp); err != nil {
		return nil, err
	}

	embeddings := make([]EmbeddingData, len(vResp.Data))
	for i, d := range vResp.Data {
		embeddings[i] = EmbeddingData{
			Index:     d.Index,
			Embedding: d.Embedding,
		}
	}

	return &EmbeddingResponse{
		Provider:   p.Name(),
		Model:      vResp.Model,
		Embeddings: embeddings,
		Usage: EmbeddingUsage{
			TotalTokens: vResp.Usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// EmbedQuery embeds a single query.
func (p *VoyageProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return p.BaseProvider.EmbedQuery(ctx, query, p.Embed)
}

// EmbedDocuments embeds multiple documents.
func (p *VoyageProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return p.BaseProvider.EmbedDocuments(ctx, documents, p.Embed)
}

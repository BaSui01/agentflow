package embedding

import (
	"context"
	"encoding/json"
	"time"
)

// Voyage Provider 执行使用 Voyage AI 的 API 嵌入.
type VoyageProvider struct {
	*BaseProvider
	cfg VoyageConfig
}

// NewVoyage Provider创建了一个新的Voyage AI嵌入提供商.
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

// Embed使用Voyage AI生成嵌入.
func (p *VoyageProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	model := ChooseModel(req.Model, p.cfg.Model, "voyage-3-large")

	body := voyageEmbedRequest{
		Input:    req.Input,
		Model:    model,
		Truncate: req.Truncate,
	}

	// 地图输入类型
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

// 嵌入查询嵌入了单个查询.
func (p *VoyageProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return p.BaseProvider.EmbedQuery(ctx, query, p.Embed)
}

// 嵌入文件嵌入多个文档。
func (p *VoyageProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return p.BaseProvider.EmbedDocuments(ctx, documents, p.Embed)
}

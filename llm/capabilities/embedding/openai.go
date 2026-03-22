package embedding

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// OpenAIProvider 执行使用 OpenAI 的 API 嵌入.
type OpenAIProvider struct {
	*BaseProvider
	cfg OpenAIConfig
}

// NewOpenAIProvider创建了新的OpenAI嵌入提供商.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	cfg.BaseProviderConfig = applyBaseProviderDefaults(cfg.BaseProviderConfig, "https://api.openai.com", "text-embedding-3-large")
	if cfg.Dimensions == 0 {
		cfg.Dimensions = 3072
	}

	return &OpenAIProvider{
		BaseProvider: newProviderBase("openai-embedding", cfg.BaseProviderConfig, cfg.Dimensions, 2048),
		cfg:          cfg,
	}
}

type openAIEmbedRequest struct {
	Input          any    `json:"input"`
	Model          string `json:"model"`
	Dimensions     int    `json:"dimensions,omitempty"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	InputType      string `json:"input_type,omitempty"`
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

// 嵌入为给定输入生成嵌入.
func (p *OpenAIProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	if err := validateEmbeddingRequest(req, p.Name()); err != nil {
		return nil, err
	}

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
	if req.InputType != "" {
		body.InputType = string(req.InputType)
	}

	respBody, err := p.DoRequest(ctx, "POST", "/v1/embeddings", body, map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}

	var oaResp openAIEmbedResponse
	if err := json.Unmarshal(respBody, &oaResp); err != nil {
		return nil, fmt.Errorf("failed to decode openai embedding response: %w", err)
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

// 嵌入查询嵌入了单个查询.
func (p *OpenAIProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return p.BaseProvider.EmbedQuery(ctx, query, p.Embed)
}

// 嵌入文件嵌入多个文档。
func (p *OpenAIProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return p.BaseProvider.EmbedDocuments(ctx, documents, p.Embed)
}

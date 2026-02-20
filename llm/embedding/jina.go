package embedding

import (
	"context"
	"encoding/json"
	"time"
)

// Jina Provider 执行使用 Jina AI 的 API 嵌入.
type JinaProvider struct {
	*BaseProvider
	cfg JinaConfig
}

// 新JinaProvider创建了新的Jina AI嵌入服务商.
func NewJinaProvider(cfg JinaConfig) *JinaProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.jina.ai"
	}
	if cfg.Model == "" {
		cfg.Model = "jina-embeddings-v3"
	}

	return &JinaProvider{
		BaseProvider: NewBaseProvider(BaseConfig{
			Name:       "jina-embedding",
			BaseURL:    cfg.BaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.Model,
			Dimensions: 1024,
			MaxBatch:   2048,
			Timeout:    cfg.Timeout,
		}),
		cfg: cfg,
	}
}

type jinaEmbedRequest struct {
	Input         []string `json:"input"`
	Model         string   `json:"model"`
	Task          string   `json:"task,omitempty"`       // retrieval.query, retrieval.passage, etc.
	Dimensions    int      `json:"dimensions,omitempty"` // Matryoshka dimensions
	LateChunking  bool     `json:"late_chunking,omitempty"`
	EmbeddingType string   `json:"embedding_type,omitempty"` // float, binary, ubinary
}

type jinaEmbedResponse struct {
	Model  string `json:"model"`
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		TotalTokens  int `json:"total_tokens"`
		PromptTokens int `json:"prompt_tokens"`
	} `json:"usage"`
}

// 嵌入会使用Jina AI生成嵌入.
func (p *JinaProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	model := ChooseModel(req.Model, p.cfg.Model, "jina-embeddings-v3")

	body := jinaEmbedRequest{
		Input: req.Input,
		Model: model,
	}

	// 映射 Jina 任务的输入类型
	switch req.InputType {
	case InputTypeQuery:
		body.Task = "retrieval.query"
	case InputTypeDocument:
		body.Task = "retrieval.passage"
	case InputTypeClassify:
		body.Task = "classification"
	case InputTypeClustering:
		body.Task = "text-matching"
	case InputTypeCodeQuery:
		body.Task = "retrieval.query"
	case InputTypeCodeDoc:
		body.Task = "retrieval.passage"
	}

	// 支持 Matryoshka 维度
	if req.Dimensions > 0 {
		body.Dimensions = req.Dimensions
	}

	respBody, err := p.DoRequest(ctx, "POST", "/v1/embeddings", body, map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}

	var jResp jinaEmbedResponse
	if err := json.Unmarshal(respBody, &jResp); err != nil {
		return nil, err
	}

	embeddings := make([]EmbeddingData, len(jResp.Data))
	for i, d := range jResp.Data {
		embeddings[i] = EmbeddingData{
			Index:     d.Index,
			Embedding: d.Embedding,
		}
	}

	return &EmbeddingResponse{
		Provider:   p.Name(),
		Model:      jResp.Model,
		Embeddings: embeddings,
		Usage: EmbeddingUsage{
			PromptTokens: jResp.Usage.PromptTokens,
			TotalTokens:  jResp.Usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// 嵌入查询嵌入了单个查询.
func (p *JinaProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return p.BaseProvider.EmbedQuery(ctx, query, p.Embed)
}

// 嵌入文件嵌入多个文档。
func (p *JinaProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return p.BaseProvider.EmbedDocuments(ctx, documents, p.Embed)
}

package embedding

import (
	"context"
	"encoding/json"
	"time"
)

// Cohere Provider 执行使用 Cohere API的嵌入.
type CohereProvider struct {
	*BaseProvider
	cfg CohereConfig
}

// NewCohere Provider创建了一个新的Cohere嵌入提供商.
func NewCohereProvider(cfg CohereConfig) *CohereProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.cohere.ai"
	}
	if cfg.Model == "" {
		cfg.Model = "embed-v3.5"
	}

	return &CohereProvider{
		BaseProvider: NewBaseProvider(BaseConfig{
			Name:       "cohere-embedding",
			BaseURL:    cfg.BaseURL,
			APIKey:     cfg.APIKey,
			Model:      cfg.Model,
			Dimensions: 1024,
			MaxBatch:   96,
			Timeout:    cfg.Timeout,
		}),
		cfg: cfg,
	}
}

type cohereEmbedRequest struct {
	Texts         []string `json:"texts"`
	Model         string   `json:"model"`
	InputType     string   `json:"input_type"`                // search_query, search_document, classification, clustering
	Truncate      string   `json:"truncate,omitempty"`        // NONE, START, END
	EmbeddingType []string `json:"embedding_types,omitempty"` // float, int8, uint8, binary, ubinary
}

type cohereEmbedResponse struct {
	ID         string `json:"id"`
	Embeddings struct {
		Float [][]float64 `json:"float"`
	} `json:"embeddings"`
	Texts []string `json:"texts"`
	Meta  struct {
		APIVersion struct {
			Version string `json:"version"`
		} `json:"api_version"`
		BilledUnits struct {
			InputTokens int `json:"input_tokens"`
		} `json:"billed_units"`
	} `json:"meta"`
}

// 嵌入会使用Cohere生成嵌入.
func (p *CohereProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	model := ChooseModel(req.Model, p.cfg.Model, "embed-v3.5")

	body := cohereEmbedRequest{
		Texts:         req.Input,
		Model:         model,
		EmbeddingType: []string{"float"},
	}

	// 地图输入类型
	switch req.InputType {
	case InputTypeQuery:
		body.InputType = "search_query"
	case InputTypeDocument:
		body.InputType = "search_document"
	case InputTypeClassify:
		body.InputType = "classification"
	case InputTypeClustering:
		body.InputType = "clustering"
	default:
		body.InputType = "search_document"
	}

	if req.Truncate {
		body.Truncate = "END"
	}

	respBody, err := p.DoRequest(ctx, "POST", "/v2/embed", body, map[string]string{
		"Authorization": "Bearer " + p.cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}

	var cResp cohereEmbedResponse
	if err := json.Unmarshal(respBody, &cResp); err != nil {
		return nil, err
	}

	embeddings := make([]EmbeddingData, len(cResp.Embeddings.Float))
	for i, emb := range cResp.Embeddings.Float {
		embeddings[i] = EmbeddingData{
			Index:     i,
			Embedding: emb,
		}
	}

	return &EmbeddingResponse{
		ID:         cResp.ID,
		Provider:   p.Name(),
		Model:      model,
		Embeddings: embeddings,
		Usage: EmbeddingUsage{
			PromptTokens: cResp.Meta.BilledUnits.InputTokens,
			TotalTokens:  cResp.Meta.BilledUnits.InputTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// 嵌入查询嵌入了单个查询.
func (p *CohereProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return p.BaseProvider.EmbedQuery(ctx, query, p.Embed)
}

// 嵌入文件嵌入多个文档。
func (p *CohereProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return p.BaseProvider.EmbedDocuments(ctx, documents, p.Embed)
}

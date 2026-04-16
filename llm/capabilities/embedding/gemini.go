package embedding

import (
	"context"
	"fmt"
	"time"

	googlegenai "github.com/BaSui01/agentflow/llm/internal/googlegenai"
	"github.com/BaSui01/agentflow/llm/providers"
	"google.golang.org/genai"
)

// GeminiProvider 使用 Google Gemini API 执行嵌入.
// 注: Gemini 使用不同的端点格式: /models/{model}:embedContent
type GeminiProvider struct {
	*BaseProvider
	cfg GeminiConfig
}

// GeminiConfig 配置 Gemini 嵌入提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、BaseURL、Model、Timeout 字段。
type GeminiConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultGeminiConfig 返回默认 Gemini 嵌入配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://generativelanguage.googleapis.com/v1beta",
			Model:   "gemini-embedding-001",
			Timeout: 30 * time.Second,
		},
	}
}

// NewGeminiProvider 创建新的 Gemini 嵌入提供者.
func NewGeminiProvider(cfg GeminiConfig) *GeminiProvider {
	cfg.BaseProviderConfig = applyBaseProviderDefaults(cfg.BaseProviderConfig, "https://generativelanguage.googleapis.com/v1beta", "gemini-embedding-001")

	return &GeminiProvider{
		BaseProvider: newProviderBase("gemini-embedding", cfg.BaseProviderConfig, 3072, 100),
		cfg:          cfg,
	}
}

// Gemini TaskType 映射
type geminiTaskType string

const (
	geminiTaskRetrievalQuery    geminiTaskType = "RETRIEVAL_QUERY"
	geminiTaskRetrievalDocument geminiTaskType = "RETRIEVAL_DOCUMENT"
	geminiTaskSemantic          geminiTaskType = "SEMANTIC_SIMILARITY"
	geminiTaskClassification    geminiTaskType = "CLASSIFICATION"
	geminiTaskClustering        geminiTaskType = "CLUSTERING"
	geminiTaskCodeRetrieval     geminiTaskType = "CODE_RETRIEVAL_QUERY"
)

type geminiEmbedRequest struct {
	Model                string         `json:"model"`
	Content              geminiContent  `json:"content"`
	TaskType             geminiTaskType `json:"taskType,omitempty"`
	Title                string         `json:"title,omitempty"`
	OutputDimensionality int            `json:"outputDimensionality,omitempty"`
}

type geminiBatchEmbedRequest struct {
	Requests []geminiEmbedRequest `json:"requests"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiEmbedResponse struct {
	Embedding geminiContentEmbedding `json:"embedding"`
}

type geminiBatchEmbedResponse struct {
	Embeddings []geminiContentEmbedding `json:"embeddings"`
}

type geminiContentEmbedding struct {
	Values []float64 `json:"values"`
}

// mapTaskType 将输入任务类型转换为 Gemini 任务类型.
func mapTaskType(inputType InputType) geminiTaskType {
	switch inputType {
	case InputTypeQuery:
		return geminiTaskRetrievalQuery
	case InputTypeDocument:
		return geminiTaskRetrievalDocument
	case InputTypeClassify:
		return geminiTaskClassification
	case InputTypeClustering:
		return geminiTaskClustering
	case InputTypeCodeQuery, InputTypeCodeDoc:
		return geminiTaskCodeRetrieval
	default:
		return geminiTaskRetrievalDocument
	}
}

// Embed 使用 Gemini API 生成嵌入.
func (p *GeminiProvider) Embed(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	if err := validateEmbeddingRequest(req, p.Name()); err != nil {
		return nil, err
	}

	model := ChooseModel(req.Model, p.cfg.Model, "gemini-embedding-001")
	taskType := mapTaskType(req.InputType)
	client, err := googlegenai.NewClient(ctx, googlegenai.ClientConfig{
		APIKey:  p.cfg.APIKey,
		BaseURL: p.cfg.BaseURL,
		Timeout: p.cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create google genai client: %w", err)
	}

	contents := make([]*genai.Content, 0, len(req.Input))
	for _, text := range req.Input {
		contents = append(contents, genai.NewContentFromText(text, genai.RoleUser))
	}

	cfg := &genai.EmbedContentConfig{
		TaskType: string(taskType),
	}
	if req.Dimensions > 0 {
		dims := int32(req.Dimensions)
		cfg.OutputDimensionality = &dims
	}
	if req.Truncate {
		cfg.AutoTruncate = true
	}

	resp, err := client.Models.EmbedContent(ctx, model, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("gemini embed request failed: %w", err)
	}

	embeddings := make([]EmbeddingData, 0, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		if emb == nil {
			continue
		}
		vector := make([]float64, 0, len(emb.Values))
		for _, value := range emb.Values {
			vector = append(vector, float64(value))
		}
		embeddings = append(embeddings, EmbeddingData{
			Index:     i,
			Embedding: vector,
		})
	}

	return &EmbeddingResponse{
		Provider:   p.Name(),
		Model:      model,
		Embeddings: embeddings,
		CreatedAt:  time.Now(),
	}, nil
}

// EmbedQuery 嵌入单个查询.
func (p *GeminiProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return p.BaseProvider.EmbedQuery(ctx, query, p.Embed)
}

// EmbedDocuments 嵌入多个文档.
func (p *GeminiProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return p.BaseProvider.EmbedDocuments(ctx, documents, p.Embed)
}

package embedding

import (
	"github.com/BaSui01/agentflow/types"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// GeminiProvider 使用 Google Gemini API 执行嵌入.
// 注: Gemini 使用不同的端点格式: /models/{model}:embedContent
type GeminiProvider struct {
	*BaseProvider
	cfg    GeminiConfig
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
	if len(req.Input) == 0 {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    "input must not be empty",
			HTTPStatus: 400,
			Retryable:  false,
			Provider:   p.Name(),
		}
	}
	model := ChooseModel(req.Model, p.cfg.Model, "gemini-embedding-001")
	taskType := mapTaskType(req.InputType)

	// 对多个输入使用批量端点
	if len(req.Input) > 1 {
		return p.batchEmbed(ctx, req, model, taskType)
	}

	// 单嵌入
	body := geminiEmbedRequest{
		Model: fmt.Sprintf("models/%s", model),
		Content: geminiContent{
			Parts: []geminiPart{{Text: req.Input[0]}},
		},
		TaskType: taskType,
	}
	if req.Dimensions > 0 {
		body.OutputDimensionality = req.Dimensions
	}

	endpoint := fmt.Sprintf("/models/%s:embedContent", model)
	respBody, err := p.DoRequest(ctx, "POST", endpoint, body, map[string]string{
		"x-goog-api-key": p.cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}

	var gResp geminiEmbedResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	return &EmbeddingResponse{
		Provider: p.Name(),
		Model:    model,
		Embeddings: []EmbeddingData{{
			Index:     0,
			Embedding: gResp.Embedding.Values,
		}},
		CreatedAt: time.Now(),
	}, nil
}

// 批量 Embed 处理批量嵌入请求。
func (p *GeminiProvider) batchEmbed(ctx context.Context, req *EmbeddingRequest, model string, taskType geminiTaskType) (*EmbeddingResponse, error) {
	requests := make([]geminiEmbedRequest, len(req.Input))
	for i, text := range req.Input {
		requests[i] = geminiEmbedRequest{
			Model: fmt.Sprintf("models/%s", model),
			Content: geminiContent{
				Parts: []geminiPart{{Text: text}},
			},
			TaskType: taskType,
		}
		if req.Dimensions > 0 {
			requests[i].OutputDimensionality = req.Dimensions
		}
	}

	body := geminiBatchEmbedRequest{Requests: requests}
	endpoint := fmt.Sprintf("/models/%s:batchEmbedContents", model)

	respBody, err := p.DoRequest(ctx, "POST", endpoint, body, map[string]string{
		"x-goog-api-key": p.cfg.APIKey,
	})
	if err != nil {
		return nil, err
	}

	var gResp geminiBatchEmbedResponse
	if err := json.Unmarshal(respBody, &gResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini batch response: %w", err)
	}

	embeddings := make([]EmbeddingData, len(gResp.Embeddings))
	for i, emb := range gResp.Embeddings {
		embeddings[i] = EmbeddingData{
			Index:     i,
			Embedding: emb.Values,
		}
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



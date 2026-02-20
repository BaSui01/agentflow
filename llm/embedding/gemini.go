package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GeminiProvider 使用 Google Gemini API 执行嵌入.
// 注: Gemini 使用不同的端点格式: /models/{model}:embedContent
type GeminiProvider struct {
	cfg    GeminiConfig
	client *http.Client
}

// GeminiConfig 配置 Gemini 嵌入提供者.
type GeminiConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // gemini-embedding-001
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultGeminiConfig 返回默认 Gemini 嵌入配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		BaseURL: "https://generativelanguage.googleapis.com/v1beta",
		Model:   "gemini-embedding-001",
		Timeout: 30 * time.Second,
	}
}

// NewGeminiProvider 创建新的 Gemini 嵌入提供者.
func NewGeminiProvider(cfg GeminiConfig) *GeminiProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if cfg.Model == "" {
		cfg.Model = "gemini-embedding-001"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &GeminiProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *GeminiProvider) Name() string      { return "gemini-embedding" }
func (p *GeminiProvider) Dimensions() int   { return 3072 }
func (p *GeminiProvider) MaxBatchSize() int { return 100 }

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

	endpoint := fmt.Sprintf("%s/models/%s:embedContent", strings.TrimRight(p.cfg.BaseURL, "/"), model)
	respBody, err := p.doRequest(ctx, endpoint, body)
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
	endpoint := fmt.Sprintf("%s/models/%s:batchEmbedContents", strings.TrimRight(p.cfg.BaseURL, "/"), model)

	respBody, err := p.doRequest(ctx, endpoint, body)
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

// doRequest 使用 Gemini 特定认证执行 HTTP 请求.
func (p *GeminiProvider) doRequest(ctx context.Context, endpoint string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Gemini 使用 x-goog-api-key 头（不是 Bearer 令牌）
	httpReq.Header.Set("x-goog-api-key", p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gemini error: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// EmbedQuery 嵌入单个查询.
func (p *GeminiProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	resp, err := p.Embed(ctx, &EmbeddingRequest{
		Input:     []string{query},
		InputType: InputTypeQuery,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return resp.Embeddings[0].Embedding, nil
}

// EmbedDocuments 嵌入多个文档.
func (p *GeminiProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	resp, err := p.Embed(ctx, &EmbeddingRequest{
		Input:     documents,
		InputType: InputTypeDocument,
	})
	if err != nil {
		return nil, err
	}
	result := make([][]float64, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		result[i] = emb.Embedding
	}
	return result, nil
}

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

	"github.com/BaSui01/agentflow/llm"
)

// BaseProvider为嵌入提供者提供了共同的功能.
type BaseProvider struct {
	name       string
	client     *http.Client
	baseURL    string
	apiKey     string
	model      string
	dimensions int
	maxBatch   int
}

// BaseConfig持有基础提供者的共同配置.
type BaseConfig struct {
	Name       string
	BaseURL    string
	APIKey     string
	Model      string
	Dimensions int
	MaxBatch   int
	Timeout    time.Duration
}

// NewBase Provider创建了一个新的基础提供者.
func NewBaseProvider(cfg BaseConfig) *BaseProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	maxBatch := cfg.MaxBatch
	if maxBatch == 0 {
		maxBatch = 100
	}
	return &BaseProvider{
		name:       cfg.Name,
		client:     &http.Client{Timeout: timeout},
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		dimensions: cfg.Dimensions,
		maxBatch:   maxBatch,
	}
}

func (p *BaseProvider) Name() string      { return p.name }
func (p *BaseProvider) Dimensions() int   { return p.dimensions }
func (p *BaseProvider) MaxBatchSize() int { return p.maxBatch }

// 嵌入查询嵌入单个查询字符串.
func (p *BaseProvider) EmbedQuery(ctx context.Context, query string, embedFn func(context.Context, *EmbeddingRequest) (*EmbeddingResponse, error)) ([]float64, error) {
	resp, err := embedFn(ctx, &EmbeddingRequest{
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

// 嵌入文件嵌入多个文档。
func (p *BaseProvider) EmbedDocuments(ctx context.Context, documents []string, embedFn func(context.Context, *EmbeddingRequest) (*EmbeddingResponse, error)) ([][]float64, error) {
	resp, err := embedFn(ctx, &EmbeddingRequest{
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

// Dorequest 执行 HTTP 请求, 并进行常见错误处理 。
func (p *BaseProvider) DoRequest(ctx context.Context, method, endpoint string, body any, headers map[string]string) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+endpoint, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.name,
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, mapHTTPError(resp.StatusCode, string(respBody), p.name)
	}

	return respBody, nil
}

// 映射 HTTPerror 映射 HTTP 状态到 llm. 错误。
func mapHTTPError(status int, msg, provider string) *llm.Error {
	code := llm.ErrUpstreamError
	retryable := status >= 500

	switch status {
	case http.StatusUnauthorized:
		code = llm.ErrUnauthorized
	case http.StatusForbidden:
		code = llm.ErrForbidden
	case http.StatusTooManyRequests:
		code = llm.ErrRateLimited
		retryable = true
	case http.StatusBadRequest:
		code = llm.ErrInvalidRequest
	}

	return &llm.Error{
		Code:       code,
		Message:    msg,
		HTTPStatus: status,
		Retryable:  retryable,
		Provider:   provider,
	}
}

// 从请求或默认中选择模式。
func ChooseModel(reqModel, defaultModel, fallback string) string {
	if reqModel != "" {
		return reqModel
	}
	if defaultModel != "" {
		return defaultModel
	}
	return fallback
}

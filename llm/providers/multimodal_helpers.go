package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/BaSui01/agentflow/llm"
)

// =============================================================================
// OpenAI 兼容 API 通用请求执行
// =============================================================================

// doOpenAICompatRequest 是 OpenAI 兼容 API 的通用 HTTP 请求执行函数。
// 它封装了 marshal -> create request -> set headers -> do -> check status -> decode 的完整流程。
func doOpenAICompatRequest[Req any, Resp any](
	ctx context.Context,
	client *http.Client,
	baseURL, apiKey, providerName, endpoint string,
	req *Req,
	buildHeadersFunc func(*http.Request, string),
) (*Resp, error) {
	fullEndpoint := fmt.Sprintf("%s%s", baseURL, endpoint)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	buildHeadersFunc(httpReq, apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := ReadErrorMessage(resp.Body)
		return nil, MapHTTPError(resp.StatusCode, msg, providerName)
	}

	var result Resp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}

	return &result, nil
}

// =============================================================================
// 图像生成助手
// =============================================================================

// GenerateImageOpenAICompat 通用的 OpenAI 兼容图像生成函数
func GenerateImageOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.ImageGenerationRequest, buildHeadersFunc func(*http.Request, string)) (*llm.ImageGenerationResponse, error) {
	return doOpenAICompatRequest[llm.ImageGenerationRequest, llm.ImageGenerationResponse](ctx, client, baseURL, apiKey, providerName, endpoint, req, buildHeadersFunc)
}

// =============================================================================
// 视频生成助手
// =============================================================================

// GenerateVideoOpenAICompat 通用的 OpenAI 兼容视频生成函数
func GenerateVideoOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.VideoGenerationRequest, buildHeadersFunc func(*http.Request, string)) (*llm.VideoGenerationResponse, error) {
	return doOpenAICompatRequest[llm.VideoGenerationRequest, llm.VideoGenerationResponse](ctx, client, baseURL, apiKey, providerName, endpoint, req, buildHeadersFunc)
}

// =============================================================================
// 音频生成助手
// =============================================================================

// GenerateAudioOpenAICompat 通用的 OpenAI 兼容音频生成函数
func GenerateAudioOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.AudioGenerationRequest, buildHeadersFunc func(*http.Request, string)) (*llm.AudioGenerationResponse, error) {
	fullEndpoint := fmt.Sprintf("%s%s", baseURL, endpoint)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	buildHeadersFunc(httpReq, apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := ReadErrorMessage(resp.Body)
		return nil, MapHTTPError(resp.StatusCode, msg, providerName)
	}

	// 读取音频数据（直接从已有的 resp.Body 读取）
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}

	return &llm.AudioGenerationResponse{
		Audio: buf.Bytes(),
	}, nil
}

// =============================================================================
// 嵌入助手
// =============================================================================

// CreateEmbeddingOpenAICompat 通用的 OpenAI 兼容 Embedding 函数
func CreateEmbeddingOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.EmbeddingRequest, buildHeadersFunc func(*http.Request, string)) (*llm.EmbeddingResponse, error) {
	return doOpenAICompatRequest[llm.EmbeddingRequest, llm.EmbeddingResponse](ctx, client, baseURL, apiKey, providerName, endpoint, req, buildHeadersFunc)
}

// =============================================================================
// 不支持功能助手
// =============================================================================

// NotSupportedError 返回不支持的错误
func NotSupportedError(providerName, feature string) *llm.Error {
	return &llm.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    fmt.Sprintf("%s is not supported by %s", feature, providerName),
		HTTPStatus: http.StatusNotImplemented,
		Provider:   providerName,
	}
}

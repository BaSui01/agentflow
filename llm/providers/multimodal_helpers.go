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
// XQ 图像生成助手
// =============================================================================

// GenerateImageOpenAICompat 通用的 OpenAI 兼容图像生成函数
func GenerateImageOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.ImageGenerationRequest, buildHeadersFunc func(*http.Request, string)) (*llm.ImageGenerationResponse, error) {
	fullEndpoint := fmt.Sprintf("%s%s", baseURL, endpoint)

	payload, _ := json.Marshal(req)
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

	var imageResp llm.ImageGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&imageResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}

	return &imageResp, nil
}

// =============================================================================
// 视频生成助手
// =============================================================================

// GenerateVideoOpenAICompat 通用的 OpenAI 兼容视频生成函数
func GenerateVideoOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.VideoGenerationRequest, buildHeadersFunc func(*http.Request, string)) (*llm.VideoGenerationResponse, error) {
	fullEndpoint := fmt.Sprintf("%s%s", baseURL, endpoint)

	payload, _ := json.Marshal(req)
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

	var videoResp llm.VideoGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&videoResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}

	return &videoResp, nil
}

// =============================================================================
// QQ 音频生成助手
// =============================================================================

// GenerateAudioOpenAICompat 通用的 OpenAI 兼容音频生成函数
func GenerateAudioOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.AudioGenerationRequest, buildHeadersFunc func(*http.Request, string)) (*llm.AudioGenerationResponse, error) {
	fullEndpoint := fmt.Sprintf("%s%s", baseURL, endpoint)

	payload, _ := json.Marshal(req)
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

	// 读取音频数据
	audioData, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}
	defer audioData.Body.Close()

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
// * 嵌入帮助者
// =============================================================================

// CreateEmbeddingOpenAICompat 通用的 OpenAI 兼容 Embedding 函数
func CreateEmbeddingOpenAICompat(ctx context.Context, client *http.Client, baseURL, apiKey, providerName, endpoint string, req *llm.EmbeddingRequest, buildHeadersFunc func(*http.Request, string)) (*llm.EmbeddingResponse, error) {
	fullEndpoint := fmt.Sprintf("%s%s", baseURL, endpoint)

	payload, _ := json.Marshal(req)
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

	var embeddingResp llm.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   providerName,
		}
	}

	return &embeddingResp, nil
}

// =============================================================================
// 未支持的帮助者
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

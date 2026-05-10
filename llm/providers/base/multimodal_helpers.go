package providerbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
)

// =============================================================================
// OpenAI 兼容 API 通用请求执行
// =============================================================================

// OpenAICompatParams 聚合了 OpenAI 兼容 API 调用所需的公共参数。
// 引入此结构体将各 Generate* 函数的参数从 8 个缩减为 1 个，
// 提升可读性并降低传参出错风险。
type OpenAICompatParams struct {
	Client           *http.Client
	BaseURL          string
	APIKey           string
	ProviderName     string
	Endpoint         string
	BuildHeadersFunc func(*http.Request, string)
}

// doOpenAICompatRequest 是 OpenAI 兼容 API 的通用 HTTP 请求执行函数。
// 它封装了 marshal -> create request -> set headers -> do -> check status -> decode 的完整流程。
func doOpenAICompatRequest[Req any, Resp any](
	ctx context.Context,
	p OpenAICompatParams,
	req *Req,
) (*Resp, error) {
	fullEndpoint := fmt.Sprintf("%s%s", p.BaseURL, p.Endpoint)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.BuildHeadersFunc(httpReq, p.APIKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.ProviderName,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := ReadErrorMessage(resp.Body)
		return nil, MapHTTPError(resp.StatusCode, msg, p.ProviderName)
	}

	var result Resp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.ProviderName,
		}
	}

	return &result, nil
}

// =============================================================================
// 图像生成助手
// =============================================================================

// GenerateImageOpenAICompat 通用的 OpenAI 兼容图像生成函数
func GenerateImageOpenAICompat(ctx context.Context, p OpenAICompatParams, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return doOpenAICompatRequest[llm.ImageGenerationRequest, llm.ImageGenerationResponse](ctx, p, req)
}

// =============================================================================
// 视频生成助手
// =============================================================================

// GenerateVideoOpenAICompat 通用的 OpenAI 兼容视频生成函数
func GenerateVideoOpenAICompat(ctx context.Context, p OpenAICompatParams, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return doOpenAICompatRequest[llm.VideoGenerationRequest, llm.VideoGenerationResponse](ctx, p, req)
}

// =============================================================================
// 音频生成助手
// =============================================================================

// GenerateAudioOpenAICompat 通用的 OpenAI 兼容音频生成函数
func GenerateAudioOpenAICompat(ctx context.Context, p OpenAICompatParams, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	fullEndpoint := fmt.Sprintf("%s%s", p.BaseURL, p.Endpoint)

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.BuildHeadersFunc(httpReq, p.APIKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.ProviderName,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := ReadErrorMessage(resp.Body)
		return nil, MapHTTPError(resp.StatusCode, msg, p.ProviderName)
	}

	// 读取音频数据（直接从已有的 resp.Body 读取）
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.ProviderName,
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
func CreateEmbeddingOpenAICompat(ctx context.Context, p OpenAICompatParams, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return doOpenAICompatRequest[llm.EmbeddingRequest, llm.EmbeddingResponse](ctx, p, req)
}

// =============================================================================
// 不支持功能助手
// =============================================================================

// NotSupportedError 返回不支持的错误
func NotSupportedError(providerName, feature string) *types.Error {
	return &types.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    fmt.Sprintf("%s is not supported by %s", feature, providerName),
		HTTPStatus: http.StatusNotImplemented,
		Provider:   providerName,
	}
}

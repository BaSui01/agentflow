package providerbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// HeaderFunc 自定义 HTTP 请求头构建函数。
type HeaderFunc func(r *http.Request, apiKey string)

// BaseCapabilityProvider 为多模态能力模块（image/video/speech/embedding/music/threed/rerank/moderation）
// 提供通用的 HTTP 客户端、认证和错误处理基础设施。
type BaseCapabilityProvider struct {
	ProviderName string
	Client       *http.Client
	BaseURL      string
	APIKey       string
	Model        string
	BuildHeaders HeaderFunc
}

// CapabilityConfig 构造 BaseCapabilityProvider 所需的配置。
type CapabilityConfig struct {
	Name         string
	BaseURL      string
	APIKey       string
	Model        string
	Timeout      time.Duration
	BuildHeaders HeaderFunc
}

// NewBaseCapabilityProvider 创建通用能力 provider 基类。
func NewBaseCapabilityProvider(cfg CapabilityConfig) *BaseCapabilityProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	headers := cfg.BuildHeaders
	if headers == nil {
		headers = BearerTokenHeaders
	}
	return &BaseCapabilityProvider{
		ProviderName: cfg.Name,
		Client:       tlsutil.SecureHTTPClient(timeout),
		BaseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		APIKey:       cfg.APIKey,
		Model:        cfg.Model,
		BuildHeaders: headers,
	}
}

// PostJSON 发送 JSON POST 请求，返回原始响应体。
// 自动处理 marshal、设置 headers、状态码检查和结构化错误映射。
func (p *BaseCapabilityProvider) PostJSON(ctx context.Context, endpoint string, body any) ([]byte, error) {
	return p.DoRaw(ctx, http.MethodPost, endpoint, body)
}

// PostJSONDecode 发送 JSON POST 请求并将响应解码到 result。
func (p *BaseCapabilityProvider) PostJSONDecode(ctx context.Context, endpoint string, body any, result any) error {
	data, err := p.PostJSON(ctx, endpoint, body)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    fmt.Sprintf("failed to decode response: %s", err),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.ProviderName,
		}
	}
	return nil
}

// GetJSON 发送 GET 请求，返回原始响应体。
func (p *BaseCapabilityProvider) GetJSON(ctx context.Context, endpoint string) ([]byte, error) {
	return p.DoRaw(ctx, http.MethodGet, endpoint, nil)
}

// GetJSONDecode 发送 GET 请求并将响应解码到 result。
func (p *BaseCapabilityProvider) GetJSONDecode(ctx context.Context, endpoint string, result any) error {
	data, err := p.GetJSON(ctx, endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, result); err != nil {
		return &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    fmt.Sprintf("failed to decode response: %s", err),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.ProviderName,
		}
	}
	return nil
}

// DoRaw 执行底层 HTTP 请求，支持 JSON 和非 JSON 场景。
// body 为 nil 时不发送请求体；body 为 io.Reader 时直接使用；否则 JSON marshal。
func (p *BaseCapabilityProvider) DoRaw(ctx context.Context, method, endpoint string, body any) ([]byte, error) {
	var reqBody io.Reader
	isJSON := true

	if body != nil {
		switch v := body.(type) {
		case io.Reader:
			reqBody = v
			isJSON = false
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request: %w", err)
			}
			reqBody = bytes.NewReader(data)
		}
	}

	url := p.BaseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	p.BuildHeaders(req, p.APIKey)
	// 仅在 JSON body 时覆盖 Content-Type（multipart 等场景由调用方自行设置）
	if body != nil && isJSON {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.ProviderName,
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := ReadErrorMessage(bytes.NewReader(respBody))
		return nil, MapHTTPError(resp.StatusCode, msg, p.ProviderName)
	}

	return respBody, nil
}

// ChooseCapabilityModel 根据请求模型和默认模型选择最终使用的模型。
func ChooseCapabilityModel(reqModel, defaultModel string) string {
	if reqModel != "" {
		return reqModel
	}
	return defaultModel
}

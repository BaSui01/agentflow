package doubao

import (
	"bytes"
	"io"
	"net/http"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"go.uber.org/zap"
)

// DoubaoProvider 实现字节跳动豆包 LLM 提供者.
// Doubao 使用 OpenAI 兼容的 API 格式.
type DoubaoProvider struct {
	*openaicompat.Provider
}

// NewDoubaoProvider 创建新的 Doubao 提供者实例.
func NewDoubaoProvider(cfg providers.DoubaoConfig, logger *zap.Logger) *DoubaoProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://ark.cn-beijing.volces.com"
	}

	var buildHeaders func(*http.Request, string)
	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		signer := newVolcSigner(cfg.AccessKey, cfg.SecretKey, cfg.Region)
		buildHeaders = func(req *http.Request, _ string) {
			req.Header.Set("Content-Type", "application/json")
			// 计算 body hash
			bodyHash := hashSHA256("")
			if req.Body != nil {
				// 读取 body 计算 hash，然后重置
				bodyBytes, err := io.ReadAll(req.Body)
				if err == nil {
					bodyHash = hashSHA256(string(bodyBytes))
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
			}
			signer.sign(req, bodyHash)
		}
	}

	return &DoubaoProvider{
		Provider: openaicompat.New(openaicompat.Config{
			ProviderName:  "doubao",
			APIKey:        cfg.APIKey,
			APIKeys:       cfg.APIKeys,
			BaseURL:       cfg.BaseURL,
			DefaultModel:  cfg.Model,
			FallbackModel: "Doubao-1.5-pro-32k",
			Timeout:       cfg.Timeout,
			EndpointPath:  "/api/v3/chat/completions",
			RequestHook:   doubaoRequestHook,
			BuildHeaders:  buildHeaders,
		}, logger),
	}
}

// doubaoRequestHook 处理豆包特有的请求参数。
// 支持 Thinking（推理模式）参数映射。
func doubaoRequestHook(req *llm.ChatRequest, body *providerbase.OpenAICompatRequest) {
	if req.ReasoningMode != "" {
		switch req.ReasoningMode {
		case "thinking", "enabled":
			body.Thinking = &providerbase.Thinking{Type: "enabled"}
		case "disabled":
			body.Thinking = &providerbase.Thinking{Type: "disabled"}
		case "auto":
			body.Thinking = &providerbase.Thinking{Type: "auto"}
		}
	}
}

package llm

import (
	"context"
	"fmt"
)

// ProverWrapper 用动态 API 密钥和 BaseURL 支持包装 Prover
type ProviderWrapper struct {
	baseProvider Provider
	apiKey       string
	baseURL      string
}

// NewProviderWrapper 创建了新的提供者包装器
func NewProviderWrapper(baseProvider Provider, apiKey, baseURL string) *ProviderWrapper {
	return &ProviderWrapper{
		baseProvider: baseProvider,
		apiKey:       apiKey,
		baseURL:      baseURL,
	}
}

// 提供方接口
func (w *ProviderWrapper) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 必要时将 API 密钥和 BaseURL 输入请求上下文
	// 这是简化执行----实际执行取决于提供者
	return w.baseProvider.Completion(ctx, req)
}

// Stream 设备提供接口
func (w *ProviderWrapper) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	return w.baseProvider.Stream(ctx, req)
}

// 健康检查工具
func (w *ProviderWrapper) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return w.baseProvider.HealthCheck(ctx)
}

// 名称工具 提供者接口
func (w *ProviderWrapper) Name() string {
	return w.baseProvider.Name()
}

// 支持 NativeFunctionCalling 工具 提供者接口
func (w *ProviderWrapper) SupportsNativeFunctionCalling() bool {
	return w.baseProvider.SupportsNativeFunctionCalling()
}

// ListModels 执行提供者接口 。
func (w *ProviderWrapper) ListModels(ctx context.Context) ([]Model, error) {
	return w.baseProvider.ListModels(ctx)
}

// GetAPIKey 返回 API 密钥
func (w *ProviderWrapper) GetAPIKey() string {
	return w.apiKey
}

// GetBaseURL 返回基址
func (w *ProviderWrapper) GetBaseURL() string {
	return w.baseURL
}

// API 键和 BaseURL 创建提供者实例
type ProviderFactory interface {
	CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error)
}

// 默认执行
type DefaultProviderFactory struct {
	constructors map[string]func(apiKey, baseURL string) (Provider, error)
}

// 新建默认提供者工厂
func NewDefaultProviderFactory() *DefaultProviderFactory {
	return &DefaultProviderFactory{
		constructors: make(map[string]func(apiKey, baseURL string) (Provider, error)),
	}
}

// 提供方 注册 提供方 构造器
func (f *DefaultProviderFactory) RegisterProvider(code string, constructor func(apiKey, baseURL string) (Provider, error)) {
	f.constructors[code] = constructor
}

// 创建提供者实例
func (f *DefaultProviderFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error) {
	constructor, exists := f.constructors[providerCode]
	if !exists {
		return nil, fmt.Errorf("provider %s not registered", providerCode)
	}
	
	return constructor(apiKey, baseURL)
}

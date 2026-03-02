package router

import (
	"fmt"
	"sync"
)

// ProviderFactory 定义 runtime/router 内部使用的 provider 构造接口。
type ProviderFactory interface {
	CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error)
}

// DefaultProviderFactory 是 ProviderFactory 的线程安全默认实现。
type DefaultProviderFactory struct {
	mu           sync.RWMutex
	constructors map[string]func(apiKey, baseURL string) (Provider, error)
}

// NewDefaultProviderFactory 创建默认工厂实现。
func NewDefaultProviderFactory() *DefaultProviderFactory {
	return &DefaultProviderFactory{
		constructors: make(map[string]func(apiKey, baseURL string) (Provider, error)),
	}
}

// RegisterProvider 注册 provider 构造器。
func (f *DefaultProviderFactory) RegisterProvider(code string, constructor func(apiKey, baseURL string) (Provider, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.constructors[code] = constructor
}

// CreateProvider 根据 provider code 创建实例。
func (f *DefaultProviderFactory) CreateProvider(providerCode string, apiKey string, baseURL string) (Provider, error) {
	f.mu.RLock()
	constructor, exists := f.constructors[providerCode]
	f.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("provider %s not registered", providerCode)
	}
	return constructor(apiKey, baseURL)
}

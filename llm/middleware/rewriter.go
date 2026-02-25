package middleware

import (
	"context"
	"fmt"
	"sync"

	llmpkg "github.com/BaSui01/agentflow/llm"
)

// RequestRewriter 请求改写器接口
// 用于在请求发送到上游 API 之前进行参数清理和转换
type RequestRewriter interface {
	// Rewrite 改写请求
	// 返回改写后的请求和错误（如果改写失败）
	Rewrite(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatRequest, error)

	// Name 返回改写器名称（用于日志和调试）
	Name() string
}

// RewriterChain 改写器链
// 按顺序执行多个改写器，线程安全
type RewriterChain struct {
	mu        sync.RWMutex
	rewriters []RequestRewriter
}

// NewRewriterChain 创建改写器链
func NewRewriterChain(rewriters ...RequestRewriter) *RewriterChain {
	return &RewriterChain{
		rewriters: rewriters,
	}
}

// Execute 执行改写器链
// 按顺序执行所有改写器，任何一个失败则中断并返回错误
func (c *RewriterChain) Execute(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatRequest, error) {
	if c == nil {
		return req, nil
	}

	c.mu.RLock()
	rewriters := make([]RequestRewriter, len(c.rewriters))
	copy(rewriters, c.rewriters)
	c.mu.RUnlock()

	if len(rewriters) == 0 {
		return req, nil
	}

	var err error
	for _, rewriter := range rewriters {
		req, err = rewriter.Rewrite(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("rewriter [%s] failed: %w", rewriter.Name(), err)
		}
	}

	return req, nil
}

// AddRewriter 动态添加改写器
func (c *RewriterChain) AddRewriter(rewriter RequestRewriter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rewriters = append(c.rewriters, rewriter)
}

// GetRewriters 获取所有改写器（用于调试）
func (c *RewriterChain) GetRewriters() []RequestRewriter {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]RequestRewriter, len(c.rewriters))
	copy(out, c.rewriters)
	return out
}

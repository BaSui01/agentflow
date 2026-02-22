package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ThoughtSignature 表示一个加密推理签名.
type ThoughtSignature struct {
	ID        string                 `json:"id"`
	Signature string                 `json:"signature"`
	Model     string                 `json:"model"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt time.Time              `json:"expires_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// ThoughtChain 表示一组推理签名序列.
type ThoughtChain struct {
	ID         string             `json:"id"`
	Signatures []ThoughtSignature `json:"signatures"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

// ThoughtSignatureManager 管理思维签名以维持推理连续性.
type ThoughtSignatureManager struct {
	chains map[string]*ThoughtChain
	mu     sync.RWMutex
	ttl    time.Duration
}

// NewThoughtSignatureManager 创建新的签名管理器.
func NewThoughtSignatureManager(ttl time.Duration) *ThoughtSignatureManager {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &ThoughtSignatureManager{
		chains: make(map[string]*ThoughtChain),
		ttl:    ttl,
	}
}

// CreateChain 创建一条新的思维链.
func (m *ThoughtSignatureManager) CreateChain(id string) *ThoughtChain {
	m.mu.Lock()
	defer m.mu.Unlock()

	chain := &ThoughtChain{
		ID:         id,
		Signatures: make([]ThoughtSignature, 0),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	m.chains[id] = chain
	return chain
}

// GetChain 获取指定 ID 的思维链.
func (m *ThoughtSignatureManager) GetChain(id string) *ThoughtChain {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.chains[id]
}

// AddSignature 将签名添加到指定的链中。
func (m *ThoughtSignatureManager) AddSignature(chainID string, sig ThoughtSignature) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	chain, ok := m.chains[chainID]
	if !ok {
		return fmt.Errorf("chain not found: %s", chainID)
	}

	if sig.ExpiresAt.IsZero() {
		sig.ExpiresAt = time.Now().Add(m.ttl)
	}

	chain.Signatures = append(chain.Signatures, sig)
	chain.UpdatedAt = time.Now()
	return nil
}

// GetLatestSignatures 返回链中最新的签名。
func (m *ThoughtSignatureManager) GetLatestSignatures(chainID string, count int) []ThoughtSignature {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chain, ok := m.chains[chainID]
	if !ok {
		return nil
	}

	now := time.Now()
	var valid []ThoughtSignature
	for _, sig := range chain.Signatures {
		if sig.ExpiresAt.After(now) {
			valid = append(valid, sig)
		}
	}

	if count > 0 && len(valid) > count {
		return valid[len(valid)-count:]
	}
	return valid
}

// CleanExpired 清除过期的签名。
func (m *ThoughtSignatureManager) CleanExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for _, chain := range m.chains {
		var valid []ThoughtSignature
		for _, sig := range chain.Signatures {
			if sig.ExpiresAt.After(now) {
				valid = append(valid, sig)
			}
		}
		chain.Signatures = valid
	}
}

// ThoughtSignatureMiddleware 包装提供者以处理思维签名.
type ThoughtSignatureMiddleware struct {
	provider Provider
	manager  *ThoughtSignatureManager
}

// NewThoughtSignatureMiddleware 创建新的思维签名中间件.
func NewThoughtSignatureMiddleware(provider Provider, manager *ThoughtSignatureManager) *ThoughtSignatureMiddleware {
	return &ThoughtSignatureMiddleware{
		provider: provider,
		manager:  manager,
	}
}

// Completion 用思维签名处理包装提供者的补全调用.
func (m *ThoughtSignatureMiddleware) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 从请求元数据中提取链式ID
	chainID := ""
	if req.Metadata != nil {
		chainID = req.Metadata["thought_chain_id"]
	}

	// 在请求中添加先前的签名
	if chainID != "" {
		sigs := m.manager.GetLatestSignatures(chainID, 5)
		if len(sigs) > 0 {
			sigStrings := make([]string, len(sigs))
			for i, s := range sigs {
				sigStrings[i] = s.Signature
			}
			req.ThoughtSignatures = sigStrings
		}
	}

	// 调用提供者
	resp, err := m.provider.Completion(ctx, req)
	if err != nil {
		return nil, err
	}

	// 从响应中存储新签名
	if chainID != "" && len(resp.ThoughtSignatures) > 0 {
		for _, sigStr := range resp.ThoughtSignatures {
			sig := ThoughtSignature{
				ID:        generateSignatureID(sigStr),
				Signature: sigStr,
				Model:     req.Model,
				CreatedAt: time.Now(),
			}
			m.manager.AddSignature(chainID, sig)
		}
	}

	return resp, nil
}

// Stream 用思维签名处理包装流式调用.
func (m *ThoughtSignatureMiddleware) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	// 在请求中添加签名
	chainID := ""
	if req.Metadata != nil {
		chainID = req.Metadata["thought_chain_id"]
	}

	if chainID != "" {
		sigs := m.manager.GetLatestSignatures(chainID, 5)
		if len(sigs) > 0 {
			sigStrings := make([]string, len(sigs))
			for i, s := range sigs {
				sigStrings[i] = s.Signature
			}
			req.ThoughtSignatures = sigStrings
		}
	}

	return m.provider.Stream(ctx, req)
}

// Name 返回提供者名称。
func (m *ThoughtSignatureMiddleware) Name() string {
	return m.provider.Name()
}

// HealthCheck 委托给被包装的提供者。
func (m *ThoughtSignatureMiddleware) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return m.provider.HealthCheck(ctx)
}

// SupportsNativeFunctionCalling 委托给被包装的提供者.
func (m *ThoughtSignatureMiddleware) SupportsNativeFunctionCalling() bool {
	return m.provider.SupportsNativeFunctionCalling()
}

// ListModels 委托给被包装的提供者.
func (m *ThoughtSignatureMiddleware) ListModels(ctx context.Context) ([]Model, error) {
	return m.provider.ListModels(ctx)
}

// Endpoints 委托给被包装的提供者。
func (m *ThoughtSignatureMiddleware) Endpoints() ProviderEndpoints {
	return m.provider.Endpoints()
}

func generateSignatureID(sig string) string {
	hash := sha256.Sum256([]byte(sig + time.Now().String()))
	return hex.EncodeToString(hash[:8])
}

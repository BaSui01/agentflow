// package llm为维持推理链连续性提供思想签名支持.
// 思想签名是OpenAI/Gemini用于保存的加密推理签名
// 跨越多个API呼叫的推理上下文.
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ThoughtSignature代表一个加密推理签名.
type ThoughtSignature struct {
	ID        string                 `json:"id"`
	Signature string                 `json:"signature"`
	Model     string                 `json:"model"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt time.Time              `json:"expires_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ThoughtChain代表了一系列推理签名.
type ThoughtChain struct {
	ID         string             `json:"id"`
	Signatures []ThoughtSignature `json:"signatures"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

// ThoughtSignatureManager 管理思想签名用于推理连续性.
type ThoughtSignatureManager struct {
	chains map[string]*ThoughtChain
	mu     sync.RWMutex
	ttl    time.Duration
}

// NewThoughtSignatureManager创建了新管理器.
func NewThoughtSignatureManager(ttl time.Duration) *ThoughtSignatureManager {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &ThoughtSignatureManager{
		chains: make(map[string]*ThoughtChain),
		ttl:    ttl,
	}
}

// CreateChain创建了一个新的思维链.
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

// 让钱找到一个思想链
func (m *ThoughtSignatureManager) GetChain(id string) *ThoughtChain {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.chains[id]
}

// 添加签名将签名添加到链中。
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

// GetLatestSignatures 返回链条的最新签名 。
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

// CleanExpired 删除过期的签名 。
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

// ThoughtSignatureMiddleware 包装一个提供者来处理思想签名.
type ThoughtSignatureMiddleware struct {
	provider Provider
	manager  *ThoughtSignatureManager
}

// NewThought SignatureMiddleware 创建了新的中间软件.
func NewThoughtSignatureMiddleware(provider Provider, manager *ThoughtSignatureManager) *ThoughtSignatureMiddleware {
	return &ThoughtSignatureMiddleware{
		provider: provider,
		manager:  manager,
	}
}

// 完成将提供者的完成用思维签名处理包裹.
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

	// 电话提供者
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

// 串流与思维签名处理相接。
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

// 名称返回提供者名称 。
func (m *ThoughtSignatureMiddleware) Name() string {
	return m.provider.Name()
}

// 健康检查代表 到包装提供者。
func (m *ThoughtSignatureMiddleware) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return m.provider.HealthCheck(ctx)
}

// 支持NativeFunction Call代表到包裹供应商.
func (m *ThoughtSignatureMiddleware) SupportsNativeFunctionCalling() bool {
	return m.provider.SupportsNativeFunctionCalling()
}

// ListModels代表到包裹提供者.
func (m *ThoughtSignatureMiddleware) ListModels(ctx context.Context) ([]Model, error) {
	return m.provider.ListModels(ctx)
}

func generateSignatureID(sig string) string {
	hash := sha256.Sum256([]byte(sig + time.Now().String()))
	return hex.EncodeToString(hash[:8])
}

// Package llm provides Thought Signatures support for maintaining reasoning chain continuity.
// Thought Signatures are encrypted reasoning signatures used by OpenAI/Gemini to preserve
// reasoning context across multiple API calls.
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ThoughtSignature represents an encrypted reasoning signature.
type ThoughtSignature struct {
	ID        string                 `json:"id"`
	Signature string                 `json:"signature"`
	Model     string                 `json:"model"`
	CreatedAt time.Time              `json:"created_at"`
	ExpiresAt time.Time              `json:"expires_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ThoughtChain represents a chain of reasoning signatures.
type ThoughtChain struct {
	ID         string             `json:"id"`
	Signatures []ThoughtSignature `json:"signatures"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}

// ThoughtSignatureManager manages thought signatures for reasoning continuity.
type ThoughtSignatureManager struct {
	chains map[string]*ThoughtChain
	mu     sync.RWMutex
	ttl    time.Duration
}

// NewThoughtSignatureManager creates a new manager.
func NewThoughtSignatureManager(ttl time.Duration) *ThoughtSignatureManager {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &ThoughtSignatureManager{
		chains: make(map[string]*ThoughtChain),
		ttl:    ttl,
	}
}

// CreateChain creates a new thought chain.
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

// GetChain retrieves a thought chain.
func (m *ThoughtSignatureManager) GetChain(id string) *ThoughtChain {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.chains[id]
}

// AddSignature adds a signature to a chain.
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

// GetLatestSignatures returns the latest signatures for a chain.
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

// CleanExpired removes expired signatures.
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

// ThoughtSignatureMiddleware wraps a provider to handle thought signatures.
type ThoughtSignatureMiddleware struct {
	provider Provider
	manager  *ThoughtSignatureManager
}

// NewThoughtSignatureMiddleware creates a new middleware.
func NewThoughtSignatureMiddleware(provider Provider, manager *ThoughtSignatureManager) *ThoughtSignatureMiddleware {
	return &ThoughtSignatureMiddleware{
		provider: provider,
		manager:  manager,
	}
}

// Completion wraps the provider's Completion with thought signature handling.
func (m *ThoughtSignatureMiddleware) Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// Extract chain ID from request metadata
	chainID := ""
	if req.Metadata != nil {
		chainID = req.Metadata["thought_chain_id"]
	}

	// Add previous signatures to request
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

	// Call provider
	resp, err := m.provider.Completion(ctx, req)
	if err != nil {
		return nil, err
	}

	// Store new signatures from response
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

// Stream wraps streaming with thought signature handling.
func (m *ThoughtSignatureMiddleware) Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
	// Add signatures to request
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

// Name returns the provider name.
func (m *ThoughtSignatureMiddleware) Name() string {
	return m.provider.Name()
}

// HealthCheck delegates to the wrapped provider.
func (m *ThoughtSignatureMiddleware) HealthCheck(ctx context.Context) (*HealthStatus, error) {
	return m.provider.HealthCheck(ctx)
}

// SupportsNativeFunctionCalling delegates to the wrapped provider.
func (m *ThoughtSignatureMiddleware) SupportsNativeFunctionCalling() bool {
	return m.provider.SupportsNativeFunctionCalling()
}

// ListModels delegates to the wrapped provider.
func (m *ThoughtSignatureMiddleware) ListModels(ctx context.Context) ([]Model, error) {
	return m.provider.ListModels(ctx)
}

func generateSignatureID(sig string) string {
	hash := sha256.Sum256([]byte(sig + time.Now().String()))
	return hex.EncodeToString(hash[:8])
}

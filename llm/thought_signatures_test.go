package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThoughtSignatureMiddleware_Completion_NoChainID(t *testing.T) {
	inner := &testProvider{
		name: "inner",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			return &ChatResponse{Model: req.Model, Choices: []ChatChoice{{Message: Message{Content: "ok"}}}}, nil
		},
	}
	mgr := NewThoughtSignatureManager(time.Hour)
	mw := NewThoughtSignatureMiddleware(inner, mgr)

	resp, err := mw.Completion(context.Background(), &ChatRequest{Model: "m"})
	require.NoError(t, err)
	assert.Equal(t, "ok", resp.Choices[0].Message.Content)
}

func TestThoughtSignatureMiddleware_Completion_WithChainID(t *testing.T) {
	mgr := NewThoughtSignatureManager(time.Hour)
	mgr.CreateChain("chain-1")
	mgr.AddSignature("chain-1", ThoughtSignature{
		ID:        "s1",
		Signature: "prev-sig",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	var capturedReq *ChatRequest
	inner := &testProvider{
		name: "inner",
		completionFn: func(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
			capturedReq = req
			return &ChatResponse{
				Model:             req.Model,
				ThoughtSignatures: []string{"new-sig"},
				Choices:           []ChatChoice{{Message: Message{Content: "ok"}}},
			}, nil
		},
	}

	mw := NewThoughtSignatureMiddleware(inner, mgr)
	resp, err := mw.Completion(context.Background(), &ChatRequest{
		Model:    "m",
		Metadata: map[string]string{"thought_chain_id": "chain-1"},
	})
	require.NoError(t, err)
	assert.NotNil(t, resp)

	// Previous signatures should be injected into request
	assert.Contains(t, capturedReq.ThoughtSignatures, "prev-sig")

	// New signature from response should be stored
	chain := mgr.GetChain("chain-1")
	assert.Len(t, chain.Signatures, 2) // original + new
}

func TestThoughtSignatureMiddleware_Completion_Error(t *testing.T) {
	inner := &testProvider{name: "inner"} // completionFn is nil -> returns error
	mgr := NewThoughtSignatureManager(time.Hour)
	mw := NewThoughtSignatureMiddleware(inner, mgr)

	_, err := mw.Completion(context.Background(), &ChatRequest{Model: "m"})
	require.Error(t, err)
}

func TestThoughtSignatureMiddleware_Stream(t *testing.T) {
	ch := make(chan StreamChunk)
	close(ch)
	inner := &testProvider{
		name: "inner",
		streamFn: func(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error) {
			return ch, nil
		},
	}
	mgr := NewThoughtSignatureManager(time.Hour)
	mgr.CreateChain("c1")
	mgr.AddSignature("c1", ThoughtSignature{
		ID: "s1", Signature: "sig", ExpiresAt: time.Now().Add(time.Hour),
	})

	mw := NewThoughtSignatureMiddleware(inner, mgr)
	result, err := mw.Stream(context.Background(), &ChatRequest{
		Model:    "m",
		Metadata: map[string]string{"thought_chain_id": "c1"},
	})
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestThoughtSignatureMiddleware_Delegates(t *testing.T) {
	inner := &testProvider{
		name:           "delegate-test",
		supportsNative: true,
	}
	mgr := NewThoughtSignatureManager(time.Hour)
	mw := NewThoughtSignatureMiddleware(inner, mgr)

	assert.Equal(t, "delegate-test", mw.Name())
	assert.True(t, mw.SupportsNativeFunctionCalling())

	hs, err := mw.HealthCheck(context.Background())
	require.NoError(t, err)
	assert.True(t, hs.Healthy)

	models, err := mw.ListModels(context.Background())
	require.NoError(t, err)
	assert.Nil(t, models)

	ep := mw.Endpoints()
	assert.Equal(t, ProviderEndpoints{}, ep)
}

func TestGenerateSignatureID(t *testing.T) {
	id1 := generateSignatureID("test-sig")
	id2 := generateSignatureID("test-sig")
	// IDs include time.Now() so they should differ
	assert.NotEmpty(t, id1)
	assert.Len(t, id1, 16) // hex of 8 bytes
}

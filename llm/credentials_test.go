package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// CredentialOverride tests
// =============================================================================

func TestCredentialOverride_String(t *testing.T) {
	tests := []struct {
		name string
		c    CredentialOverride
		want string
	}{
		{
			name: "empty",
			c:    CredentialOverride{},
			want: "CredentialOverride{}",
		},
		{
			name: "with api key",
			c:    CredentialOverride{APIKey: "sk-123"},
			want: "CredentialOverride{APIKey:***, SecretKey:***}",
		},
		{
			name: "with secret key",
			c:    CredentialOverride{SecretKey: "secret"},
			want: "CredentialOverride{APIKey:***, SecretKey:***}",
		},
		{
			name: "with both",
			c:    CredentialOverride{APIKey: "sk-123", SecretKey: "secret"},
			want: "CredentialOverride{APIKey:***, SecretKey:***}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.c.String())
		})
	}
}

func TestCredentialOverride_MarshalJSON(t *testing.T) {
	t.Run("empty masks nothing", func(t *testing.T) {
		c := CredentialOverride{}
		data, err := c.MarshalJSON()
		require.NoError(t, err)
		var m map[string]string
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Empty(t, m["api_key"])
		assert.Empty(t, m["secret_key"])
	})

	t.Run("masks api key", func(t *testing.T) {
		c := CredentialOverride{APIKey: "real-key"}
		data, err := c.MarshalJSON()
		require.NoError(t, err)
		var m map[string]string
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Equal(t, "***", m["api_key"])
		assert.Empty(t, m["secret_key"])
	})

	t.Run("masks both", func(t *testing.T) {
		c := CredentialOverride{APIKey: "key", SecretKey: "secret"}
		data, err := c.MarshalJSON()
		require.NoError(t, err)
		var m map[string]string
		require.NoError(t, json.Unmarshal(data, &m))
		assert.Equal(t, "***", m["api_key"])
		assert.Equal(t, "***", m["secret_key"])
	})
}

func TestWithCredentialOverride(t *testing.T) {
	t.Run("empty credential does not modify context", func(t *testing.T) {
		ctx := context.Background()
		newCtx := WithCredentialOverride(ctx, CredentialOverride{})
		assert.Equal(t, ctx, newCtx) // same context returned
		_, ok := CredentialOverrideFromContext(newCtx)
		assert.False(t, ok)
	})

	t.Run("non-empty credential stored in context", func(t *testing.T) {
		ctx := context.Background()
		c := CredentialOverride{APIKey: "test-key"}
		newCtx := WithCredentialOverride(ctx, c)
		assert.NotEqual(t, ctx, newCtx)
		got, ok := CredentialOverrideFromContext(newCtx)
		assert.True(t, ok)
		assert.Equal(t, "test-key", got.APIKey)
	})
}

func TestCredentialOverrideFromContext(t *testing.T) {
	t.Run("no credential in context", func(t *testing.T) {
		_, ok := CredentialOverrideFromContext(context.Background())
		assert.False(t, ok)
	})

	t.Run("credential present", func(t *testing.T) {
		ctx := WithCredentialOverride(context.Background(), CredentialOverride{SecretKey: "s"})
		c, ok := CredentialOverrideFromContext(ctx)
		assert.True(t, ok)
		assert.Equal(t, "s", c.SecretKey)
	})
}

// =============================================================================
// ThoughtSignatureManager tests
// =============================================================================

func TestThoughtSignatureManager_CreateAndGetChain(t *testing.T) {
	mgr := NewThoughtSignatureManager(time.Hour)

	chain := mgr.CreateChain("chain-1")
	assert.NotNil(t, chain)
	assert.Equal(t, "chain-1", chain.ID)
	assert.Empty(t, chain.Signatures)

	got := mgr.GetChain("chain-1")
	assert.Equal(t, chain, got)

	assert.Nil(t, mgr.GetChain("nonexistent"))
}

func TestThoughtSignatureManager_AddSignature(t *testing.T) {
	mgr := NewThoughtSignatureManager(time.Hour)
	mgr.CreateChain("c1")

	err := mgr.AddSignature("c1", ThoughtSignature{
		ID:        "sig-1",
		Signature: "abc",
		Model:     "gpt-4",
	})
	require.NoError(t, err)

	chain := mgr.GetChain("c1")
	assert.Len(t, chain.Signatures, 1)
	assert.Equal(t, "sig-1", chain.Signatures[0].ID)
	// ExpiresAt should be set automatically
	assert.False(t, chain.Signatures[0].ExpiresAt.IsZero())
}

func TestThoughtSignatureManager_AddSignature_ChainNotFound(t *testing.T) {
	mgr := NewThoughtSignatureManager(time.Hour)
	err := mgr.AddSignature("nonexistent", ThoughtSignature{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chain not found")
}

func TestThoughtSignatureManager_GetLatestSignatures(t *testing.T) {
	mgr := NewThoughtSignatureManager(time.Hour)
	mgr.CreateChain("c1")

	for i := 0; i < 10; i++ {
		mgr.AddSignature("c1", ThoughtSignature{
			ID:        "sig",
			Signature: "s",
			Model:     "m",
		})
	}

	// Get last 3
	sigs := mgr.GetLatestSignatures("c1", 3)
	assert.Len(t, sigs, 3)

	// Get all (count=0)
	all := mgr.GetLatestSignatures("c1", 0)
	assert.Len(t, all, 10)

	// Nonexistent chain
	assert.Nil(t, mgr.GetLatestSignatures("nope", 5))
}

func TestThoughtSignatureManager_CleanExpired(t *testing.T) {
	mgr := NewThoughtSignatureManager(time.Millisecond)
	mgr.CreateChain("c1")

	mgr.AddSignature("c1", ThoughtSignature{
		ID:        "expired",
		Signature: "s",
		ExpiresAt: time.Now().Add(-time.Hour), // already expired
	})
	mgr.AddSignature("c1", ThoughtSignature{
		ID:        "valid",
		Signature: "s",
		ExpiresAt: time.Now().Add(time.Hour), // still valid
	})

	mgr.CleanExpired()

	chain := mgr.GetChain("c1")
	assert.Len(t, chain.Signatures, 1)
	assert.Equal(t, "valid", chain.Signatures[0].ID)
}

func TestThoughtSignatureManager_DefaultTTL(t *testing.T) {
	mgr := NewThoughtSignatureManager(0) // should default to 24h
	assert.Equal(t, 24*time.Hour, mgr.ttl)
}


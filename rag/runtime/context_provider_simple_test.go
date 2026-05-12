package runtime

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSimpleContextProviderGenerateContextUsesMetadataAndCaches(t *testing.T) {
	provider := NewSimpleContextProvider(zap.NewNop())
	doc := Document{ID: "doc-1", Metadata: map[string]any{"title": "Handbook", "section": "Install"}}
	chunk := strings.Repeat("This chunk explains setup steps. ", 8)

	first, err := provider.GenerateContext(context.Background(), doc, chunk)
	require.NoError(t, err)
	assert.Contains(t, first, `document titled "Handbook"`)
	assert.Contains(t, first, `section "Install"`)
	assert.Contains(t, first, "covering:")
	assert.Contains(t, first, "...")

	provider.mu.Lock()
	provider.cache[providerCacheKey(doc.ID, chunk)] = "cached context"
	provider.mu.Unlock()
	second, err := provider.GenerateContext(context.Background(), doc, chunk)
	require.NoError(t, err)
	assert.Equal(t, "cached context", second)
}

func TestSimpleContextProviderFallbackAndCanceledContext(t *testing.T) {
	provider := NewSimpleContextProvider(nil)
	got, err := provider.GenerateContext(context.Background(), Document{ID: "doc-2"}, "   ")
	require.NoError(t, err)
	assert.Equal(t, "General content chunk", got)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = provider.GenerateContext(ctx, Document{ID: "doc-3"}, "content")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestTruncateTextKeepsWordBoundaryWhenPossible(t *testing.T) {
	assert.Equal(t, "short", truncateText(" short ", 10))
	assert.Equal(t, "alpha beta...", truncateText("alpha beta gamma delta", 12))
	assert.Equal(t, "abcdefghij...", truncateText("abcdefghijklmnop", 10))
}

func providerCacheKey(docID, chunk string) string {
	return docID + ":" + uint64String(hashString(chunk))
}

func uint64String(v uint64) string {
	return fmt.Sprintf("%d", v)
}

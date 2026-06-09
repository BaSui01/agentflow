package runtime

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQdrantStoreDefaultsAndStablePointID(t *testing.T) {
	store := NewQdrantStore(QdrantConfig{Collection: "docs", APIKey: "secret"}, nil)

	assert.Equal(t, "http://localhost:6333", store.baseURL)
	assert.Equal(t, "Cosine", store.cfg.Distance)
	assert.Equal(t, "content", store.cfg.PayloadContentField)
	assert.Equal(t, "metadata", store.cfg.PayloadMetadataField)
	assert.Equal(t, "doc_id", store.cfg.PayloadIDField)
	require.NotNil(t, store.cfg.Wait)
	assert.True(t, *store.cfg.Wait)

	assert.Equal(t, qdrantPointID("doc-1"), qdrantPointID("doc-1"))
	assert.NotEqual(t, qdrantPointID("doc-1"), qdrantPointID("doc-2"))
}

func TestQdrantStoreApplyHeaders(t *testing.T) {
	store := NewQdrantStore(QdrantConfig{Collection: "docs", APIKey: "secret"}, nil)
	req, err := http.NewRequest(http.MethodGet, "http://example.test", nil)
	require.NoError(t, err)

	store.applyHeaders(req)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "secret", req.Header.Get("api-key"))
}

func TestPineconeStoreDefaultsAndEnsureBaseURLValidation(t *testing.T) {
	store := NewPineconeStore(PineconeConfig{}, nil)
	assert.Equal(t, "https://api.pinecone.io", store.cfg.ControllerBaseURL)
	assert.Equal(t, "content", store.cfg.MetadataContentField)
	assert.Empty(t, store.baseURL)

	err := store.ensureBaseURL(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pinecone base_url is required")

	withURL := NewPineconeStore(PineconeConfig{BaseURL: "https://example.test/"}, nil)
	require.NoError(t, withURL.ensureBaseURL(t.Context()))
	assert.Equal(t, "https://example.test", withURL.baseURL)
}

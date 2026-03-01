package rerank

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Config tests ---

func TestDefaultCohereConfig(t *testing.T) {
	cfg := DefaultCohereConfig()
	assert.Equal(t, "https://api.cohere.ai", cfg.BaseURL)
	assert.Equal(t, "rerank-v3.5", cfg.Model)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestDefaultJinaConfig(t *testing.T) {
	cfg := DefaultJinaConfig()
	assert.Equal(t, "https://api.jina.ai", cfg.BaseURL)
	assert.Equal(t, "jina-reranker-v2-base-multilingual", cfg.Model)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

func TestDefaultVoyageConfig(t *testing.T) {
	cfg := DefaultVoyageConfig()
	assert.Equal(t, "https://api.voyageai.com", cfg.BaseURL)
	assert.Equal(t, "rerank-2", cfg.Model)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

// --- Cohere Provider tests ---

func TestNewCohereProvider(t *testing.T) {
	p := NewCohereProvider(CohereConfig{APIKey: "test-key"})
	assert.Equal(t, "cohere-rerank", p.Name())
	assert.Equal(t, 1000, p.MaxDocuments())
	assert.Equal(t, "https://api.cohere.ai", p.cfg.BaseURL)
	assert.Equal(t, "rerank-v3.5", p.cfg.Model)
}

func TestCohereProvider_Rerank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/rerank", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req cohereRerankRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "test query", req.Query)
		assert.Equal(t, 2, len(req.Documents))

		resp := cohereRerankResponse{
			ID: "resp-123",
			Results: []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Document       *struct {
					Text string `json:"text"`
				} `json:"document,omitempty"`
			}{
				{Index: 1, RelevanceScore: 0.95},
				{Index: 0, RelevanceScore: 0.80},
			},
		}
		resp.Meta.BilledUnits.SearchUnits = 1
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	p := NewCohereProvider(CohereConfig{APIKey: "test-key", BaseURL: srv.URL})
	result, err := p.Rerank(context.Background(), &RerankRequest{
		Query: "test query",
		Documents: []Document{
			{Text: "doc one", ID: "d1"},
			{Text: "doc two", ID: "d2"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "cohere-rerank", result.Provider)
	assert.Equal(t, 2, len(result.Results))
	assert.Equal(t, 0.95, result.Results[0].RelevanceScore)
	assert.Equal(t, 1, result.Usage.SearchUnits)
}

func TestCohereProvider_Rerank_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, err := w.Write([]byte(`{"message":"bad request"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	p := NewCohereProvider(CohereConfig{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.Rerank(context.Background(), &RerankRequest{
		Query:     "test",
		Documents: []Document{{Text: "doc"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cohere rerank error")
}

func TestCohereProvider_RerankSimple(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := cohereRerankResponse{
			ID: "resp-456",
			Results: []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Document       *struct {
					Text string `json:"text"`
				} `json:"document,omitempty"`
			}{
				{Index: 0, RelevanceScore: 0.9},
			},
		}
		err := json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	p := NewCohereProvider(CohereConfig{APIKey: "test-key", BaseURL: srv.URL})
	results, err := p.RerankSimple(context.Background(), "query", []string{"doc1"}, 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 0.9, results[0].RelevanceScore)
}

// --- Jina Provider tests ---

func TestNewJinaProvider(t *testing.T) {
	p := NewJinaProvider(JinaConfig{APIKey: "test-key"})
	assert.Equal(t, "jina-rerank", p.Name())
	assert.Equal(t, 1024, p.MaxDocuments())
}

func TestJinaProvider_Rerank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/rerank", r.URL.Path)

		resp := jinaRerankResponse{
			Model: "jina-reranker-v2",
			Results: []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Document       *struct {
					Text string `json:"text"`
				} `json:"document,omitempty"`
			}{
				{Index: 0, RelevanceScore: 0.88},
			},
		}
		resp.Usage.TotalTokens = 42
		err := json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	p := NewJinaProvider(JinaConfig{APIKey: "test-key", BaseURL: srv.URL})
	result, err := p.Rerank(context.Background(), &RerankRequest{
		Query:     "query",
		Documents: []Document{{Text: "doc", ID: "d1"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "jina-rerank", result.Provider)
	assert.Equal(t, 42, result.Usage.TotalTokens)
	assert.Len(t, result.Results, 1)
}

func TestJinaProvider_Rerank_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte(`{"error":"internal"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	p := NewJinaProvider(JinaConfig{APIKey: "test-key", BaseURL: srv.URL})
	_, err := p.Rerank(context.Background(), &RerankRequest{
		Query:     "q",
		Documents: []Document{{Text: "d"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "jina rerank error")
}

func TestJinaProvider_RerankSimple(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := jinaRerankResponse{
			Model: "jina-reranker-v2",
			Results: []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Document       *struct {
					Text string `json:"text"`
				} `json:"document,omitempty"`
			}{
				{Index: 0, RelevanceScore: 0.7},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewJinaProvider(JinaConfig{APIKey: "test-key", BaseURL: srv.URL})
	results, err := p.RerankSimple(context.Background(), "q", []string{"d"}, 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// --- Voyage Provider tests ---

func TestNewVoyageProvider(t *testing.T) {
	p := NewVoyageProvider(VoyageConfig{APIKey: "test-key"})
	assert.Equal(t, "voyage-rerank", p.Name())
	assert.Equal(t, 1000, p.MaxDocuments())
}

func TestVoyageProvider_Rerank(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/rerank", r.URL.Path)

		resp := voyageRerankResponse{
			Object: "list",
			Model:  "rerank-2",
			Data: []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Document       string  `json:"document,omitempty"`
			}{
				{Index: 0, RelevanceScore: 0.92, Document: "doc text"},
			},
		}
		resp.Usage.TotalTokens = 55
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVoyageProvider(VoyageConfig{APIKey: "test-key", BaseURL: srv.URL})
	result, err := p.Rerank(context.Background(), &RerankRequest{
		Query:           "query",
		Documents:       []Document{{Text: "doc text", ID: "d1"}},
		ReturnDocuments: true,
	})
	require.NoError(t, err)
	assert.Equal(t, "voyage-rerank", result.Provider)
	assert.Equal(t, 55, result.Usage.TotalTokens)
	assert.Len(t, result.Results, 1)
	assert.Equal(t, "doc text", result.Results[0].Document.Text)
}

func TestVoyageProvider_Rerank_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, err := w.Write([]byte(`{"error":"unauthorized"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(srv.Close)

	p := NewVoyageProvider(VoyageConfig{APIKey: "bad-key", BaseURL: srv.URL})
	_, err := p.Rerank(context.Background(), &RerankRequest{
		Query:     "q",
		Documents: []Document{{Text: "d"}},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "voyage rerank error")
}

func TestVoyageProvider_RerankSimple(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := voyageRerankResponse{
			Model: "rerank-2",
			Data: []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
				Document       string  `json:"document,omitempty"`
			}{
				{Index: 0, RelevanceScore: 0.5},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	p := NewVoyageProvider(VoyageConfig{APIKey: "test-key", BaseURL: srv.URL})
	results, err := p.RerankSimple(context.Background(), "q", []string{"d"}, 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

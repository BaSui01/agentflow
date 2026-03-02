package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ChooseModel ---

func TestChooseModel(t *testing.T) {
	assert.Equal(t, "req-model", ChooseModel("req-model", "default", "fallback"))
	assert.Equal(t, "default", ChooseModel("", "default", "fallback"))
	assert.Equal(t, "fallback", ChooseModel("", "", "fallback"))
}

// --- BaseProvider ---

func TestNewBaseProvider(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		bp := NewBaseProvider(BaseConfig{
			Name:    "test",
			BaseURL: "http://example.com/",
		})
		assert.Equal(t, "test", bp.Name())
		assert.Equal(t, 100, bp.MaxBatchSize())
		// BaseURL trailing slash trimmed
		assert.Equal(t, "http://example.com", bp.baseURL)
	})

	t.Run("custom values", func(t *testing.T) {
		bp := NewBaseProvider(BaseConfig{
			Name:       "custom",
			BaseURL:    "http://api.test",
			Dimensions: 512,
			MaxBatch:   50,
			Timeout:    10 * time.Second,
		})
		assert.Equal(t, 512, bp.Dimensions())
		assert.Equal(t, 50, bp.MaxBatchSize())
	})
}

func TestNewBaseProviderNilTimeout(t *testing.T) {
	bp := NewBaseProvider(BaseConfig{
		Name:    "zero-timeout",
		BaseURL: "http://example.com",
		Timeout: 0,
	})
	// Should use default timeout, not panic
	assert.NotNil(t, bp)
	assert.Equal(t, "zero-timeout", bp.Name())
}

func TestBaseProviderDimensionsDefault(t *testing.T) {
	bp := NewBaseProvider(BaseConfig{Name: "test", BaseURL: "http://example.com"})
	// Default dimensions should be 0 or a sensible default
	assert.GreaterOrEqual(t, bp.Dimensions(), 0)
}

func TestBaseProviderMaxBatchDefault(t *testing.T) {
	bp := NewBaseProvider(BaseConfig{Name: "test", BaseURL: "http://example.com"})
	assert.Equal(t, 100, bp.MaxBatchSize())
}

// --- mapHTTPError ---

func TestMapHTTPError(t *testing.T) {
	tests := []struct {
		status    int
		wantCode  string
		retryable bool
	}{
		{http.StatusUnauthorized, "UNAUTHORIZED", false},
		{http.StatusForbidden, "FORBIDDEN", false},
		{http.StatusTooManyRequests, "RATE_LIMIT", true},
		{http.StatusBadRequest, "INVALID_REQUEST", false},
		{http.StatusInternalServerError, "UPSTREAM_ERROR", true},
		{http.StatusBadGateway, "UPSTREAM_ERROR", true},
		{http.StatusServiceUnavailable, "UPSTREAM_ERROR", true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			err := mapHTTPError(tt.status, "test error", "test-provider")
			assert.Equal(t, tt.wantCode, string(err.Code))
			assert.Equal(t, tt.retryable, err.Retryable)
			assert.Equal(t, "test-provider", err.Provider)
			assert.Equal(t, tt.status, err.HTTPStatus)
		})
	}
}

// --- BaseProvider.DoRequest ---

func TestBaseProviderDoRequest(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()

		bp := NewBaseProvider(BaseConfig{
			Name:    "test",
			BaseURL: srv.URL,
			APIKey:  "test-key",
		})

		body, err := bp.DoRequest(context.Background(), "POST", "/embed", map[string]string{"q": "hello"}, map[string]string{
			"Authorization": "Bearer test-key",
		})
		require.NoError(t, err)
		assert.Contains(t, string(body), `"ok":true`)
	})

	t.Run("HTTP error mapped", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"invalid key"}`))
		}))
		defer srv.Close()

		bp := NewBaseProvider(BaseConfig{Name: "test", BaseURL: srv.URL})
		_, err := bp.DoRequest(context.Background(), "POST", "/embed", nil, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key")
	})

	t.Run("nil body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		bp := NewBaseProvider(BaseConfig{Name: "test", BaseURL: srv.URL})
		body, err := bp.DoRequest(context.Background(), "GET", "/health", nil, nil)
		require.NoError(t, err)
		assert.Equal(t, `{}`, string(body))
	})
}

// --- BaseProvider.EmbedQuery / EmbedDocuments ---

func TestBaseProviderEmbedQueryAndDocuments(t *testing.T) {
	mockEmbed := func(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
		embeddings := make([]EmbeddingData, len(req.Input))
		for i := range req.Input {
			embeddings[i] = EmbeddingData{Index: i, Embedding: []float64{0.1, 0.2}}
		}
		return &EmbeddingResponse{Embeddings: embeddings}, nil
	}

	bp := NewBaseProvider(BaseConfig{Name: "test", BaseURL: "http://unused"})

	t.Run("EmbedQuery", func(t *testing.T) {
		vec, err := bp.EmbedQuery(context.Background(), "hello", mockEmbed)
		require.NoError(t, err)
		assert.Equal(t, []float64{0.1, 0.2}, vec)
	})

	t.Run("EmbedDocuments", func(t *testing.T) {
		vecs, err := bp.EmbedDocuments(context.Background(), []string{"a", "b"}, mockEmbed)
		require.NoError(t, err)
		assert.Len(t, vecs, 2)
	})

	t.Run("EmbedQuery empty response", func(t *testing.T) {
		emptyEmbed := func(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
			return &EmbeddingResponse{Embeddings: nil}, nil
		}
		_, err := bp.EmbedQuery(context.Background(), "hello", emptyEmbed)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no embeddings")
	})
}

func TestBaseProviderEmbedDocumentsEmpty(t *testing.T) {
	mockEmbed := func(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
		return &EmbeddingResponse{Embeddings: nil}, nil
	}

	bp := NewBaseProvider(BaseConfig{Name: "test", BaseURL: "http://unused"})
	vecs, err := bp.EmbedDocuments(context.Background(), []string{}, mockEmbed)
	require.NoError(t, err)
	assert.Empty(t, vecs)
}

func TestBaseProviderEmbedDocumentsError(t *testing.T) {
	mockEmbed := func(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
		return nil, fmt.Errorf("embedding service down")
	}

	bp := NewBaseProvider(BaseConfig{Name: "test", BaseURL: "http://unused"})
	_, err := bp.EmbedDocuments(context.Background(), []string{"test"}, mockEmbed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embedding service down")
}

func TestBaseProviderDoRequestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	bp := NewBaseProvider(BaseConfig{
		Name:    "test",
		BaseURL: srv.URL,
		Timeout: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := bp.DoRequest(ctx, "POST", "/embed", map[string]string{"q": "hello"}, nil)
	assert.Error(t, err)
}

// --- OpenAI Provider ---

func newOpenAITestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *OpenAIProvider) {
	t.Helper()
	srv := httptest.NewServer(handler)
	p := NewOpenAIProvider(OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: srv.URL,
			Model:   "text-embedding-3-small",
		},
	})
	return srv, p
}

func TestOpenAIProviderEmbed(t *testing.T) {
	srv, p := newOpenAITestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req openAIEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "text-embedding-3-small", req.Model)

		json.NewEncoder(w).Encode(openAIEmbedResponse{
			Object: "list",
			Model:  "text-embedding-3-small",
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}{
				{Object: "embedding", Index: 0, Embedding: []float64{0.1, 0.2, 0.3}},
			},
			Usage: struct {
				PromptTokens int `json:"prompt_tokens"`
				TotalTokens  int `json:"total_tokens"`
			}{PromptTokens: 5, TotalTokens: 5},
		})
	})
	defer srv.Close()

	resp, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input: []string{"hello world"},
	})
	require.NoError(t, err)
	assert.Equal(t, "openai-embedding", resp.Provider)
	assert.Equal(t, "text-embedding-3-small", resp.Model)
	require.Len(t, resp.Embeddings, 1)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, resp.Embeddings[0].Embedding)
	assert.Equal(t, 5, resp.Usage.PromptTokens)
}

func TestOpenAIProviderEmbedQueryAndDocuments(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(openAIEmbedResponse{
			Model: "text-embedding-3-small",
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}{
				{Index: 0, Embedding: []float64{0.5}},
			},
		})
	}
	srv, p := newOpenAITestServer(t, handler)
	defer srv.Close()

	vec, err := p.EmbedQuery(context.Background(), "test query")
	require.NoError(t, err)
	assert.Equal(t, []float64{0.5}, vec)

	vecs, err := p.EmbedDocuments(context.Background(), []string{"doc1"})
	require.NoError(t, err)
	assert.Len(t, vecs, 1)
}

func TestOpenAIProviderDefaults(t *testing.T) {
	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	assert.Equal(t, "openai-embedding", p.Name())
	assert.Equal(t, 3072, p.Dimensions())
	assert.Equal(t, 2048, p.MaxBatchSize())
}

func TestProvidersRejectEmptyInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		provider Provider
	}{
		{
			name: "openai",
			provider: NewOpenAIProvider(OpenAIConfig{
				BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"},
			}),
		},
		{
			name: "cohere",
			provider: NewCohereProvider(CohereConfig{
				BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"},
			}),
		},
		{
			name: "voyage",
			provider: NewVoyageProvider(VoyageConfig{
				BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"},
			}),
		},
		{
			name: "jina",
			provider: NewJinaProvider(JinaConfig{
				BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"},
			}),
		},
		{
			name: "gemini",
			provider: NewGeminiProvider(GeminiConfig{
				BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"},
			}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.provider.Embed(context.Background(), &EmbeddingRequest{Input: []string{}})
			require.Error(t, err)

			var providerErr *types.Error
			require.True(t, errors.As(err, &providerErr), "error should be *types.Error")
			assert.Equal(t, llm.ErrInvalidRequest, providerErr.Code)
		})
	}
}

// --- Cohere Provider ---

func TestCohereProviderEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/embed", r.URL.Path)

		var req cohereEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "search_query", req.InputType)
		assert.Equal(t, []string{"float"}, req.EmbeddingType)

		json.NewEncoder(w).Encode(cohereEmbedResponse{
			ID: "resp-1",
			Embeddings: struct {
				Float [][]float64 `json:"float"`
			}{Float: [][]float64{{0.1, 0.2}}},
			Meta: struct {
				APIVersion struct {
					Version string `json:"version"`
				} `json:"api_version"`
				BilledUnits struct {
					InputTokens int `json:"input_tokens"`
				} `json:"billed_units"`
			}{BilledUnits: struct {
				InputTokens int `json:"input_tokens"`
			}{InputTokens: 3}},
		})
	}))
	defer srv.Close()

	p := NewCohereProvider(CohereConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: srv.URL,
		},
	})

	resp, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input:     []string{"hello"},
		InputType: InputTypeQuery,
	})
	require.NoError(t, err)
	assert.Equal(t, "cohere-embedding", resp.Provider)
	assert.Equal(t, "resp-1", resp.ID)
	require.Len(t, resp.Embeddings, 1)
	assert.Equal(t, 3, resp.Usage.PromptTokens)
}

func TestCohereProviderTruncate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req cohereEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "END", req.Truncate)
		err = json.NewEncoder(w).Encode(cohereEmbedResponse{
			Embeddings: struct {
				Float [][]float64 `json:"float"`
			}{Float: [][]float64{{0.1}}},
		})
		require.NoError(t, err)
	}))
	defer srv.Close()

	p := NewCohereProvider(CohereConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input:    []string{"hello"},
		Truncate: true,
	})
	require.NoError(t, err)
}

func TestCohereProviderDefaults(t *testing.T) {
	p := NewCohereProvider(CohereConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	assert.Equal(t, "cohere-embedding", p.Name())
	assert.Equal(t, 1024, p.Dimensions())
	assert.Equal(t, 96, p.MaxBatchSize())
}

func TestCohereProviderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer srv.Close()

	p := NewCohereProvider(CohereConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Embed(context.Background(), &EmbeddingRequest{Input: []string{"test"}})
	require.Error(t, err)
}

func TestVoyageProviderDefaults(t *testing.T) {
	p := NewVoyageProvider(VoyageConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	assert.Equal(t, "voyage-embedding", p.Name())
	assert.Equal(t, 1024, p.Dimensions())
	assert.Equal(t, 128, p.MaxBatchSize())
}

func TestVoyageProviderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	p := NewVoyageProvider(VoyageConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Embed(context.Background(), &EmbeddingRequest{Input: []string{"test"}})
	require.Error(t, err)
}

// --- Jina Provider ---

func TestJinaProviderEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)

		var req jinaEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "jina-embeddings-v3", req.Model)

		err = json.NewEncoder(w).Encode(jinaEmbedResponse{
			Model: "jina-embeddings-v3",
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}{
				{Index: 0, Embedding: []float64{0.3, 0.4}},
			},
			Usage: struct {
				TotalTokens  int `json:"total_tokens"`
				PromptTokens int `json:"prompt_tokens"`
			}{TotalTokens: 4, PromptTokens: 4},
		})
		require.NoError(t, err)
	}))
	defer srv.Close()

	p := NewJinaProvider(JinaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input:     []string{"test"},
		InputType: InputTypeQuery,
	})
	require.NoError(t, err)
	assert.Equal(t, "jina-embedding", resp.Provider)
	require.Len(t, resp.Embeddings, 1)
	assert.Equal(t, []float64{0.3, 0.4}, resp.Embeddings[0].Embedding)
}

func TestJinaProviderInputTypeMapping(t *testing.T) {
	tests := []struct {
		inputType InputType
		wantTask  string
	}{
		{InputTypeQuery, "retrieval.query"},
		{InputTypeDocument, "retrieval.passage"},
		{InputTypeClassify, "classification"},
		{InputTypeClustering, "text-matching"},
		{InputTypeCodeQuery, "retrieval.query"},
		{InputTypeCodeDoc, "retrieval.passage"},
	}

	for _, tt := range tests {
		t.Run(string(tt.inputType), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req jinaEmbedRequest
				err := json.NewDecoder(r.Body).Decode(&req)
				require.NoError(t, err)
				assert.Equal(t, tt.wantTask, req.Task)
				err = json.NewEncoder(w).Encode(jinaEmbedResponse{
					Data: []struct {
						Object    string    `json:"object"`
						Index     int       `json:"index"`
						Embedding []float64 `json:"embedding"`
					}{{Index: 0, Embedding: []float64{0.1}}},
				})
				require.NoError(t, err)
			}))
			defer srv.Close()

			p := NewJinaProvider(JinaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
			_, err := p.Embed(context.Background(), &EmbeddingRequest{
				Input:     []string{"test"},
				InputType: tt.inputType,
			})
			require.NoError(t, err)
		})
	}
}

func TestJinaProviderDimensions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jinaEmbedRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, 256, req.Dimensions)
		err = json.NewEncoder(w).Encode(jinaEmbedResponse{
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}{{Index: 0, Embedding: []float64{0.1}}},
		})
		require.NoError(t, err)
	}))
	defer srv.Close()

	p := NewJinaProvider(JinaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input:      []string{"test"},
		Dimensions: 256,
	})
	require.NoError(t, err)
}

// --- Voyage Provider ---

func TestVoyageProviderEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)

		var req voyageEmbedRequest
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "voyage-3-large", req.Model)
		assert.Equal(t, "query", req.InputType)

		json.NewEncoder(w).Encode(voyageEmbedResponse{
			Model: "voyage-3-large",
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}{
				{Index: 0, Embedding: []float64{0.5, 0.6}},
			},
			Usage: struct {
				TotalTokens int `json:"total_tokens"`
			}{TotalTokens: 3},
		})
	}))
	defer srv.Close()

	p := NewVoyageProvider(VoyageConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input:     []string{"test"},
		InputType: InputTypeQuery,
	})
	require.NoError(t, err)
	assert.Equal(t, "voyage-embedding", resp.Provider)
	require.Len(t, resp.Embeddings, 1)
	assert.Equal(t, 3, resp.Usage.TotalTokens)
}

func TestVoyageProviderEmbedQueryAndDocuments(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(voyageEmbedResponse{
			Model: "voyage-3-large",
			Data: []struct {
				Object    string    `json:"object"`
				Index     int       `json:"index"`
				Embedding []float64 `json:"embedding"`
			}{
				{Index: 0, Embedding: []float64{0.5}},
			},
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	p := NewVoyageProvider(VoyageConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})

	vec, err := p.EmbedQuery(context.Background(), "test query")
	require.NoError(t, err)
	assert.Equal(t, []float64{0.5}, vec)

	vecs, err := p.EmbedDocuments(context.Background(), []string{"doc1"})
	require.NoError(t, err)
	assert.Len(t, vecs, 1)
}

func TestJinaProviderDefaults(t *testing.T) {
	p := NewJinaProvider(JinaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	assert.Equal(t, "jina-embedding", p.Name())
	assert.Equal(t, 1024, p.Dimensions())
	assert.Equal(t, 2048, p.MaxBatchSize())
}

func TestJinaProviderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	p := NewJinaProvider(JinaConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Embed(context.Background(), &EmbeddingRequest{Input: []string{"test"}})
	require.Error(t, err)
}

// --- Gemini Provider ---

func TestGeminiProviderSingleEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "embedContent")
		assert.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))

		json.NewEncoder(w).Encode(geminiEmbedResponse{
			Embedding: geminiContentEmbedding{Values: []float64{0.7, 0.8}},
		})
	}))
	defer srv.Close()

	p := NewGeminiProvider(GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: srv.URL,
			Model:   "gemini-embedding-001",
		},
	})

	resp, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input:     []string{"hello"},
		InputType: InputTypeQuery,
	})
	require.NoError(t, err)
	assert.Equal(t, "gemini-embedding", resp.Provider)
	require.Len(t, resp.Embeddings, 1)
	assert.Equal(t, []float64{0.7, 0.8}, resp.Embeddings[0].Embedding)
}

func TestGeminiProviderBatchEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "batchEmbedContents")

		json.NewEncoder(w).Encode(geminiBatchEmbedResponse{
			Embeddings: []geminiContentEmbedding{
				{Values: []float64{0.1}},
				{Values: []float64{0.2}},
			},
		})
	}))
	defer srv.Close()

	p := NewGeminiProvider(GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  "test-key",
			BaseURL: srv.URL,
		},
	})

	resp, err := p.Embed(context.Background(), &EmbeddingRequest{
		Input: []string{"doc1", "doc2"},
	})
	require.NoError(t, err)
	require.Len(t, resp.Embeddings, 2)
	assert.Equal(t, []float64{0.1}, resp.Embeddings[0].Embedding)
	assert.Equal(t, []float64{0.2}, resp.Embeddings[1].Embedding)
}

func TestGeminiProviderEmbedQueryAndDocuments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "batchEmbedContents") {
			json.NewEncoder(w).Encode(geminiBatchEmbedResponse{
				Embeddings: []geminiContentEmbedding{
					{Values: []float64{0.1}},
					{Values: []float64{0.2}},
				},
			})
		} else {
			json.NewEncoder(w).Encode(geminiEmbedResponse{
				Embedding: geminiContentEmbedding{Values: []float64{0.5}},
			})
		}
	}))
	defer srv.Close()

	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})

	vec, err := p.EmbedQuery(context.Background(), "query")
	require.NoError(t, err)
	assert.Equal(t, []float64{0.5}, vec)

	vecs, err := p.EmbedDocuments(context.Background(), []string{"a", "b"})
	require.NoError(t, err)
	assert.Len(t, vecs, 2)
}

func TestGeminiProviderHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Embed(context.Background(), &EmbeddingRequest{Input: []string{"test"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FORBIDDEN")
}

func TestGeminiProviderDefaults(t *testing.T) {
	p := NewGeminiProvider(GeminiConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	assert.Equal(t, "gemini-embedding", p.Name())
	assert.Equal(t, 3072, p.Dimensions())
	assert.Equal(t, 100, p.MaxBatchSize())
}

// --- mapTaskType ---

func TestMapTaskType(t *testing.T) {
	assert.Equal(t, geminiTaskRetrievalQuery, mapTaskType(InputTypeQuery))
	assert.Equal(t, geminiTaskRetrievalDocument, mapTaskType(InputTypeDocument))
	assert.Equal(t, geminiTaskClassification, mapTaskType(InputTypeClassify))
	assert.Equal(t, geminiTaskClustering, mapTaskType(InputTypeClustering))
	assert.Equal(t, geminiTaskCodeRetrieval, mapTaskType(InputTypeCodeQuery))
	assert.Equal(t, geminiTaskCodeRetrieval, mapTaskType(InputTypeCodeDoc))
	assert.Equal(t, geminiTaskRetrievalDocument, mapTaskType("unknown"))
}

// --- Default configs ---

func TestDefaultConfigs(t *testing.T) {
	oa := DefaultOpenAIConfig()
	assert.Equal(t, "text-embedding-3-large", oa.Model)
	assert.Equal(t, 3072, oa.Dimensions)

	vc := DefaultVoyageConfig()
	assert.Equal(t, "voyage-3-large", vc.Model)

	cc := DefaultCohereConfig()
	assert.Equal(t, "embed-v3.5", cc.Model)

	jc := DefaultJinaConfig()
	assert.Equal(t, "jina-embeddings-v3", jc.Model)

	gc := DefaultGeminiConfig()
	assert.Equal(t, "gemini-embedding-001", gc.Model)
}

// --- Error handling: server down ---

func TestProviderServerDown(t *testing.T) {
	// Use a closed server to simulate connection failure
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Embed(context.Background(), &EmbeddingRequest{Input: []string{"test"}})
	require.Error(t, err)
}

// --- Context cancellation ---

func TestProviderContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(OpenAIConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL, Timeout: 5 * time.Second}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := p.Embed(ctx, &EmbeddingRequest{Input: []string{"test"}})
	require.Error(t, err)
}

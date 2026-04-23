package adapter

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/rag/retrieval"
	rag "github.com/BaSui01/agentflow/rag/runtime"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmbedder 实现 rag.EmbeddingProvider 接口
type mockEmbedder struct {
	embeddings [][]float64
	err        error
	callCount  int
}

func (m *mockEmbedder) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	if len(m.embeddings) > 0 {
		return m.embeddings[0], nil
	}
	return []float64{0.1, 0.2, 0.3}, nil
}

func (m *mockEmbedder) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.embeddings, nil
}

func (m *mockEmbedder) Name() string {
	return "mock"
}

// mockRetriever 实现 retrieval.Retriever 接口
type mockRetriever struct {
	results []rag.RetrievalResult
	err     error
}

func (m *mockRetriever) Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]rag.RetrievalResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func TestNewRetrievalToolAdapter(t *testing.T) {
	mockR := &mockRetriever{}
	pipeline := retrieval.NewPipeline(
		retrieval.DefaultPipelineConfig(),
		nil,
		mockR,
		nil,
		nil,
	)
	embedder := &mockEmbedder{}
	adapter := NewRetrievalToolAdapter(pipeline, embedder, "test-collection")

	require.NotNil(t, adapter)
	assert.Equal(t, pipeline, adapter.Pipeline())
	assert.Equal(t, embedder, adapter.Embedder())
	assert.Equal(t, "test-collection", adapter.Collection())
}

func TestRetrievalToolAdapter_Retrieve(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		topK        int
		results     []rag.RetrievalResult
		embedErr    error
		retrieveErr error
		wantErr     bool
		wantLen     int
	}{
		{
			name:  "success with results",
			query: "test query",
			topK:  2,
			results: []rag.RetrievalResult{
				{
					Document: core.Document{
						ID:      "doc1",
						Content: "content1",
						Metadata: map[string]any{
							"source": "source1",
						},
					},
					FinalScore: 0.9,
				},
				{
					Document: core.Document{
						ID:      "doc2",
						Content: "content2",
						Metadata: map[string]any{
							"source": "source2",
						},
					},
					FinalScore: 0.8,
				},
				{
					Document: core.Document{
						ID:      "doc3",
						Content: "content3",
					},
					FinalScore: 0.7,
				},
			},
			wantErr: false,
			wantLen: 2, // topK=2
		},
		{
			name:    "empty results",
			query:   "no match",
			topK:    5,
			results: []rag.RetrievalResult{},
			wantErr: false,
			wantLen: 0,
		},
		{
			name:     "embedding error",
			query:    "test",
			topK:     5,
			embedErr: assert.AnError,
			wantErr:  true,
		},
		{
			name:        "retrieve error",
			query:       "test",
			topK:        5,
			retrieveErr: assert.AnError,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockR := &mockRetriever{
				results: tt.results,
				err:     tt.retrieveErr,
			}
			pipeline := retrieval.NewPipeline(
				retrieval.DefaultPipelineConfig(),
				nil,
				mockR,
				nil,
				nil,
			)
			embedder := &mockEmbedder{err: tt.embedErr}
			adapter := NewRetrievalToolAdapter(pipeline, embedder, "test")

			ctx := context.Background()
			records, err := adapter.Retrieve(ctx, tt.query, tt.topK)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, records, tt.wantLen)

			if tt.wantLen > 0 {
				// 验证第一个记录的内容
				assert.Equal(t, tt.results[0].Document.ID, records[0].DocID)
				assert.Equal(t, tt.results[0].Document.Content, records[0].Content)
				assert.Equal(t, 0.9, records[0].Score)
			}
		})
	}
}

func TestRetrievalToolAdapter_Retrieve_WithoutEmbedder(t *testing.T) {
	mockR := &mockRetriever{
		results: []rag.RetrievalResult{
			{
				Document: core.Document{
					ID:      "doc1",
					Content: "content1",
				},
				FinalScore: 0.9,
			},
		},
	}
	pipeline := retrieval.NewPipeline(
		retrieval.DefaultPipelineConfig(),
		nil,
		mockR,
		nil,
		nil,
	)
	// 不提供 embedder
	adapter := NewRetrievalToolAdapter(pipeline, nil, "test")

	ctx := context.Background()
	records, err := adapter.Retrieve(ctx, "test query", 5)

	require.NoError(t, err)
	assert.Len(t, records, 1)
}

func TestRetrievalToolAdapter_Retrieve_NilPipeline(t *testing.T) {
	adapter := &RetrievalToolAdapter{
		pipeline: nil,
	}

	ctx := context.Background()
	_, err := adapter.Retrieve(ctx, "query", 5)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline is nil")
}

func TestNewHybridRetrieverAdapter(t *testing.T) {
	config := rag.DefaultHybridRetrievalConfig()
	h := rag.NewHybridRetriever(config, nil)
	adapter := NewHybridRetrieverAdapter(h)

	require.NotNil(t, adapter)
	assert.Equal(t, h, adapter.Retriever())
}

func TestHybridRetrieverAdapter_Retrieve(t *testing.T) {
	// 创建一个 HybridRetriever 并添加文档
	config := rag.DefaultHybridRetrievalConfig()
	config.UseVector = false // 禁用向量检索，只用 BM25
	h := rag.NewHybridRetriever(config, nil)

	docs := []core.Document{
		{ID: "1", Content: "hello world test"},
		{ID: "2", Content: "hello go programming"},
		{ID: "3", Content: "test adapter pattern"},
	}
	err := h.IndexDocuments(docs)
	require.NoError(t, err)

	adapter := NewHybridRetrieverAdapter(h)
	ctx := context.Background()

	// 执行检索
	records, err := adapter.Retrieve(ctx, "hello world", nil)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(records), 1)

	// 验证返回的记录类型
	for _, r := range records {
		assert.NotEmpty(t, r.DocID)
		assert.NotEmpty(t, r.Content)
		assert.GreaterOrEqual(t, r.Score, 0.0)
	}
}

func TestHybridRetrieverAdapter_Retrieve_NilRetriever(t *testing.T) {
	adapter := &HybridRetrieverAdapter{
		retriever: nil,
	}

	ctx := context.Background()
	_, err := adapter.Retrieve(ctx, "query", nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "retriever is nil")
}

func TestHybridRetrieverAdapter_ConformsToInterface(t *testing.T) {
	// 验证 HybridRetrieverAdapter 实现了 workflow/steps.HybridRetriever 接口
	var _ interface {
		Retrieve(ctx context.Context, query string, queryEmbedding []float64) ([]types.RetrievalRecord, error)
	} = (*HybridRetrieverAdapter)(nil)
}

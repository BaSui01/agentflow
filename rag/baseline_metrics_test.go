package rag

import (
	"context"
	"testing"
	"time"
)

func TestRAGBaselineMetrics(t *testing.T) {
	cfg := DefaultHybridRetrievalConfig()
	cfg.UseBM25 = false
	cfg.UseVector = true
	cfg.UseReranking = false
	cfg.TopK = 3
	cfg.MinScore = 0

	retriever := NewHybridRetriever(cfg, nil)
	docs := []Document{
		{ID: "d1", Content: "golang concurrency", Embedding: []float64{1, 0, 0}},
		{ID: "d2", Content: "python scripting", Embedding: []float64{0, 1, 0}},
		{ID: "d3", Content: "database indexing", Embedding: []float64{0, 0, 1}},
		{ID: "d4", Content: "distributed systems", Embedding: []float64{0.9, 0.1, 0}},
	}
	if err := retriever.IndexDocuments(docs); err != nil {
		t.Fatalf("index docs failed: %v", err)
	}

	type sample struct {
		queryEmbedding []float64
		relevantDocID  string
	}
	samples := []sample{
		{queryEmbedding: []float64{1, 0, 0}, relevantDocID: "d1"},
		{queryEmbedding: []float64{0, 1, 0}, relevantDocID: "d2"},
		{queryEmbedding: []float64{0, 0, 1}, relevantDocID: "d3"},
		{queryEmbedding: []float64{0.95, 0.05, 0}, relevantDocID: "d1"},
	}

	var totalLatency time.Duration
	var successAtK int
	var mrr float64
	var errorsCount int

	for _, s := range samples {
		start := time.Now()
		results, err := retriever.Retrieve(context.Background(), "baseline-query", s.queryEmbedding)
		latency := time.Since(start)
		totalLatency += latency

		if err != nil {
			errorsCount++
			continue
		}

		rank := 0
		for i, r := range results {
			if r.Document.ID == s.relevantDocID {
				rank = i + 1
				break
			}
		}
		if rank > 0 && rank <= cfg.TopK {
			successAtK++
			mrr += 1.0 / float64(rank)
		}
	}

	n := float64(len(samples))
	avgLatencyMS := float64(totalLatency.Milliseconds()) / n
	recallAtK := float64(successAtK) / n
	mrr = mrr / n
	errorRate := float64(errorsCount) / n

	t.Logf("BASELINE_METRICS latency_ms=%.2f recall_at_k=%.4f mrr=%.4f error_rate=%.4f", avgLatencyMS, recallAtK, mrr, errorRate)

	if recallAtK < 0.75 {
		t.Fatalf("recall@k too low: %.4f", recallAtK)
	}
	if mrr < 0.5 {
		t.Fatalf("mrr too low: %.4f", mrr)
	}
}

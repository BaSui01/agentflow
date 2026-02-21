package rag

import (
	"testing"
)

// TestUpdateIDFStats_AvgDocLen_SingleBatch verifies that avgDocLen is correctly
// computed for a single batch of documents.
func TestUpdateIDFStats_AvgDocLen_SingleBatch(t *testing.T) {
	t.Parallel()

	cr := &ContextualRetrieval{}

	docs := []Document{
		{ID: "d1", Content: "hello world foo"},       // 3 tokens
		{ID: "d2", Content: "hello bar baz qux quux"}, // 5 tokens
	}

	cr.UpdateIDFStats(docs)

	if cr.totalDocs != 2 {
		t.Fatalf("expected totalDocs=2, got %d", cr.totalDocs)
	}

	// totalDocLen should be 3+5=8, avgDocLen = 8/2 = 4.0
	expectedAvg := 4.0
	if cr.avgDocLen != expectedAvg {
		t.Fatalf("expected avgDocLen=%f, got %f", expectedAvg, cr.avgDocLen)
	}
}

// TestUpdateIDFStats_AvgDocLen_MultipleBatches verifies that avgDocLen reflects
// the global average across multiple calls, not just the last batch.
// This is the regression test for the bug where avgDocLen was computed as
// float64(totalLen) / float64(len(docs)) instead of using the cumulative total.
func TestUpdateIDFStats_AvgDocLen_MultipleBatches(t *testing.T) {
	t.Parallel()

	cr := &ContextualRetrieval{}

	// Batch 1: 2 docs, total tokens = 3 + 5 = 8
	batch1 := []Document{
		{ID: "d1", Content: "hello world foo"},
		{ID: "d2", Content: "hello bar baz qux quux"},
	}
	cr.UpdateIDFStats(batch1)

	if cr.totalDocs != 2 {
		t.Fatalf("after batch1: expected totalDocs=2, got %d", cr.totalDocs)
	}
	if cr.avgDocLen != 4.0 {
		t.Fatalf("after batch1: expected avgDocLen=4.0, got %f", cr.avgDocLen)
	}

	// Batch 2: 1 doc, tokens = 2
	batch2 := []Document{
		{ID: "d3", Content: "single doc"},
	}
	cr.UpdateIDFStats(batch2)

	if cr.totalDocs != 3 {
		t.Fatalf("after batch2: expected totalDocs=3, got %d", cr.totalDocs)
	}

	// Global: totalDocLen = 8 + 2 = 10, totalDocs = 3, avgDocLen = 10/3 ~ 3.333
	// Before the fix, this would have been 2/1 = 2.0 (only last batch).
	expectedAvg := float64(10) / float64(3)
	if cr.avgDocLen != expectedAvg {
		t.Fatalf("after batch2: expected avgDocLen=%f (global average), got %f",
			expectedAvg, cr.avgDocLen)
	}
}

// TestUpdateIDFStats_EmptyDocs verifies that calling with empty docs is safe.
func TestUpdateIDFStats_EmptyDocs(t *testing.T) {
	t.Parallel()

	cr := &ContextualRetrieval{}
	cr.UpdateIDFStats([]Document{})

	if cr.totalDocs != 0 {
		t.Fatalf("expected totalDocs=0, got %d", cr.totalDocs)
	}
	if cr.avgDocLen != 0 {
		t.Fatalf("expected avgDocLen=0, got %f", cr.avgDocLen)
	}
}

// TestUpdateIDFStats_TotalDocLen_Accumulates verifies that totalDocLen
// accumulates correctly across batches.
func TestUpdateIDFStats_TotalDocLen_Accumulates(t *testing.T) {
	t.Parallel()

	cr := &ContextualRetrieval{}

	// 3 batches of 1 doc each, each with 2 tokens
	for i := 0; i < 3; i++ {
		cr.UpdateIDFStats([]Document{
			{ID: "d", Content: "hello world"},
		})
	}

	if cr.totalDocs != 3 {
		t.Fatalf("expected totalDocs=3, got %d", cr.totalDocs)
	}
	if cr.totalDocLen != 6 {
		t.Fatalf("expected totalDocLen=6, got %d", cr.totalDocLen)
	}
	if cr.avgDocLen != 2.0 {
		t.Fatalf("expected avgDocLen=2.0, got %f", cr.avgDocLen)
	}
}

package rag

import "testing"

func TestDefaultHybridRetrievalConfigUsesRRF(t *testing.T) {
	cfg := DefaultHybridRetrievalConfig()
	if cfg.FusionAlgorithm != FusionRRF {
		t.Fatalf("expected default fusion_algorithm rrf, got %s", cfg.FusionAlgorithm)
	}
	if cfg.RRFK <= 0 {
		t.Fatalf("expected positive rrf_k, got %d", cfg.RRFK)
	}
}

func TestMergeResultsWeightedFusion(t *testing.T) {
	r := &HybridRetriever{
		config: HybridRetrievalConfig{
			FusionAlgorithm: FusionWeighted,
			FusionAlpha:     0.8,
		},
	}
	out := r.mergeResults(
		map[string]float64{"d1": 10, "d2": 1},
		map[string]float64{"d1": 0.1, "d2": 0.9},
	)
	if out["d2"]["hybrid"] <= out["d1"]["hybrid"] {
		t.Fatalf("expected d2 > d1 under weighted alpha=0.8, got d1=%v d2=%v", out["d1"]["hybrid"], out["d2"]["hybrid"])
	}
}

func TestMergeResultsRRFFusion(t *testing.T) {
	r := &HybridRetriever{
		config: HybridRetrievalConfig{
			FusionAlgorithm: FusionRRF,
			RRFK:            60,
		},
	}
	out := r.mergeResults(
		map[string]float64{"d1": 10, "d2": 9},
		map[string]float64{"d2": 10, "d1": 9},
	)
	if out["d1"]["hybrid"] <= 0 || out["d2"]["hybrid"] <= 0 {
		t.Fatalf("expected positive rrf scores, got d1=%v d2=%v", out["d1"]["hybrid"], out["d2"]["hybrid"])
	}
}

func TestNewHybridRetriever_NormalizesFusionConfig(t *testing.T) {
	cfg := HybridRetrievalConfig{
		FusionAlgorithm: "invalid",
		FusionAlpha:     3.14,
		RRFK:            0,
	}
	r := NewHybridRetriever(cfg, nil)
	if r.config.FusionAlgorithm != FusionRRF {
		t.Fatalf("expected invalid fusion algorithm fallback to rrf, got %s", r.config.FusionAlgorithm)
	}
	if r.config.FusionAlpha != 0.5 {
		t.Fatalf("expected invalid alpha fallback to 0.5, got %f", r.config.FusionAlpha)
	}
	if r.config.RRFK != 60 {
		t.Fatalf("expected invalid rrf_k fallback to 60, got %d", r.config.RRFK)
	}
}

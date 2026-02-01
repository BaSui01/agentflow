package rag

import (
	"testing"
)

func TestDefaultCrossEncoderConfig(t *testing.T) {
	config := DefaultCrossEncoderConfig()

	if config.ModelName != "cross-encoder/ms-marco-MiniLM-L-6-v2" {
		t.Errorf("unexpected model name: %s", config.ModelName)
	}

	if config.MaxLength != 512 {
		t.Errorf("expected max length 512, got %d", config.MaxLength)
	}

	if config.BatchSize != 32 {
		t.Errorf("expected batch size 32, got %d", config.BatchSize)
	}

	if config.ScoreWeight != 0.7 {
		t.Errorf("expected score weight 0.7, got %f", config.ScoreWeight)
	}

	if config.OriginalWeight != 0.3 {
		t.Errorf("expected original weight 0.3, got %f", config.OriginalWeight)
	}
}

func TestDefaultLLMRerankerConfig(t *testing.T) {
	config := DefaultLLMRerankerConfig()

	if config.MaxCandidates == 0 {
		t.Error("expected MaxCandidates to be set")
	}

	if config.PromptTemplate == "" {
		t.Error("expected PromptTemplate to be set")
	}
}

func TestQueryDocPair(t *testing.T) {
	pair := QueryDocPair{
		Query:    "test query",
		Document: "test document content",
	}

	if pair.Query != "test query" {
		t.Errorf("expected query 'test query', got '%s'", pair.Query)
	}

	if pair.Document != "test document content" {
		t.Errorf("expected document 'test document content', got '%s'", pair.Document)
	}
}

func TestRerankerType(t *testing.T) {
	tests := []struct {
		rerankerType RerankerType
		expected     string
	}{
		{RerankerSimple, "simple"},
		{RerankerCrossEncoder, "cross_encoder"},
		{RerankerLLM, "llm"},
	}

	for _, tt := range tests {
		if string(tt.rerankerType) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, string(tt.rerankerType))
		}
	}
}

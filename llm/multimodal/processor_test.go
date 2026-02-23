package multimodal

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLLMProvider implements llm.Provider for testing MultimodalProvider.
type mockLLMProvider struct {
	name       string
	completeFn func(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error)
	streamFn   func(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error)
}

func (m *mockLLMProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if m.completeFn != nil {
		return m.completeFn(ctx, req)
	}
	return &llm.ChatResponse{}, nil
}

func (m *mockLLMProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if m.streamFn != nil {
		return m.streamFn(ctx, req)
	}
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockLLMProvider) Name() string                                          { return m.name }
func (m *mockLLMProvider) HealthCheck(_ context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}
func (m *mockLLMProvider) SupportsNativeFunctionCalling() bool { return false }
func (m *mockLLMProvider) ListModels(_ context.Context) ([]llm.Model, error) {
	return nil, nil
}
func (m *mockLLMProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{}
}

// --- MultimodalProvider tests ---

func TestNewMultimodalProvider(t *testing.T) {
	mock := &mockLLMProvider{name: "openai"}
	mp := NewMultimodalProvider(mock, nil)
	require.NotNil(t, mp)
	assert.Equal(t, "openai", mp.Name())
	// nil processor should get default
	assert.NotNil(t, mp.processor)
}

func TestNewMultimodalProvider_WithProcessor(t *testing.T) {
	mock := &mockLLMProvider{name: "openai"}
	proc := DefaultProcessor()
	mp := NewMultimodalProvider(mock, proc)
	assert.Equal(t, proc, mp.processor)
}

func TestMultimodalProvider_Completion(t *testing.T) {
	var capturedReq *llm.ChatRequest
	mock := &mockLLMProvider{
		name: "openai",
		completeFn: func(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			capturedReq = req
			return &llm.ChatResponse{Model: "gpt-4o"}, nil
		},
	}

	mp := NewMultimodalProvider(mock, nil)
	req := &MultimodalRequest{
		MultimodalMessages: []MultimodalMessage{
			{
				Role:     "user",
				Contents: []Content{NewTextContent("hello")},
			},
		},
	}

	resp, err := mp.Completion(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "gpt-4o", resp.Model)
	// Messages should have been converted and set
	require.NotNil(t, capturedReq)
	assert.Len(t, capturedReq.Messages, 1)
}

func TestMultimodalProvider_Completion_NoMultimodal(t *testing.T) {
	called := false
	mock := &mockLLMProvider{
		name: "openai",
		completeFn: func(_ context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
			called = true
			return &llm.ChatResponse{}, nil
		},
	}

	mp := NewMultimodalProvider(mock, nil)
	req := &MultimodalRequest{}
	req.ChatRequest.Messages = []llm.Message{{Role: "user", Content: "hi"}}

	_, err := mp.Completion(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, called)
}

func TestMultimodalProvider_Stream(t *testing.T) {
	mock := &mockLLMProvider{
		name: "openai",
		streamFn: func(_ context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
			ch := make(chan llm.StreamChunk, 1)
			ch <- llm.StreamChunk{Delta: llm.Message{Content: "hi"}}
			close(ch)
			return ch, nil
		},
	}

	mp := NewMultimodalProvider(mock, nil)
	req := &MultimodalRequest{
		MultimodalMessages: []MultimodalMessage{
			{Role: "user", Contents: []Content{NewTextContent("hello")}},
		},
	}

	ch, err := mp.Stream(context.Background(), req)
	require.NoError(t, err)

	var chunks []llm.StreamChunk
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	assert.Len(t, chunks, 1)
	assert.Equal(t, "hi", chunks[0].Delta.Content)
}

func TestMultimodalProvider_SupportsMultimodal(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected bool
	}{
		{"openai supports", "openai", true},
		{"anthropic supports", "anthropic", true},
		{"gemini supports", "gemini", true},
		{"unknown does not", "local-llm", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := NewMultimodalProvider(&mockLLMProvider{name: tt.provider}, nil)
			assert.Equal(t, tt.expected, mp.SupportsMultimodal())
		})
	}
}

func TestMultimodalProvider_SupportedModalities(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		expected []ContentType
	}{
		{"openai", "openai", []ContentType{ContentTypeText, ContentTypeImage, ContentTypeAudio}},
		{"anthropic", "anthropic", []ContentType{ContentTypeText, ContentTypeImage}},
		{"gemini", "gemini", []ContentType{ContentTypeText, ContentTypeImage, ContentTypeAudio, ContentTypeVideo}},
		{"unknown", "local", []ContentType{ContentTypeText}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := NewMultimodalProvider(&mockLLMProvider{name: tt.provider}, nil)
			assert.Equal(t, tt.expected, mp.SupportedModalities())
		})
	}
}

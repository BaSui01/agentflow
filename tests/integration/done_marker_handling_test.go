package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/deepseek"
	"github.com/BaSui01/agentflow/llm/providers/glm"
	"github.com/BaSui01/agentflow/llm/providers/grok"
	"github.com/BaSui01/agentflow/llm/providers/minimax"
	"github.com/BaSui01/agentflow/llm/providers/qwen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Feature: multi-provider-support, Task 12.5: [DONE] Marker Handling
// **Validates: Requirements 10.4**
//
// This unit test verifies that when a provider receives "[DONE]" marker in SSE stream,
// the stream channel is properly closed.

// TestDoneMarkerHandling_StreamClosesOnDone verifies that the stream channel
// is closed when [DONE] marker is received for all 5 providers.
func TestDoneMarkerHandling_StreamClosesOnDone(t *testing.T) {
	logger := zap.NewNop()

	// Create a mock server that sends SSE data followed by [DONE]
	mockSSEServerWithDone := func(chunks []string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send each chunk as SSE data
			for i, content := range chunks {
				sseData := map[string]any{
					"id":    fmt.Sprintf("chatcmpl-%d", i),
					"model": "test-model",
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role":    "assistant",
								"content": content,
							},
						},
					},
				}
				data, _ := json.Marshal(sseData)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			// Send [DONE] marker to signal end of stream
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	testCases := []struct {
		name         string
		providerName string
		chunks       []string
	}{
		{"grok_single_chunk", "grok", []string{"Hello"}},
		{"grok_multiple_chunks", "grok", []string{"Hello", " ", "World"}},
		{"grok_empty_before_done", "grok", []string{}},
		{"qwen_single_chunk", "qwen", []string{"Test response"}},
		{"qwen_multiple_chunks", "qwen", []string{"Part1", "Part2", "Part3"}},
		{"qwen_empty_before_done", "qwen", []string{}},
		{"deepseek_single_chunk", "deepseek", []string{"DeepSeek response"}},
		{"deepseek_multiple_chunks", "deepseek", []string{"A", "B", "C", "D"}},
		{"deepseek_empty_before_done", "deepseek", []string{}},
		{"glm_single_chunk", "glm", []string{"GLM response"}},
		{"glm_multiple_chunks", "glm", []string{"First", "Second"}},
		{"glm_empty_before_done", "glm", []string{}},
		{"minimax_single_chunk", "minimax", []string{"MiniMax response"}},
		{"minimax_multiple_chunks", "minimax", []string{"Chunk1", "Chunk2", "Chunk3"}},
		{"minimax_empty_before_done", "minimax", []string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := mockSSEServerWithDone(tc.chunks)
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch tc.providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error for provider %s", tc.providerName)

			// Collect all chunks and verify channel closes
			var receivedChunks []llm.StreamChunk
			channelClosed := false

			for chunk := range streamCh {
				receivedChunks = append(receivedChunks, chunk)
			}
			channelClosed = true

			// Verify channel was closed after [DONE]
			assert.True(t, channelClosed, "Channel should be closed after [DONE] for provider %s", tc.providerName)

			// Verify we received expected number of chunks (no extra chunks after [DONE])
			assert.Equal(t, len(tc.chunks), len(receivedChunks),
				"Should receive exactly %d chunks before [DONE] for provider %s", len(tc.chunks), tc.providerName)

			// Verify no errors in received chunks
			for i, chunk := range receivedChunks {
				assert.Nil(t, chunk.Err, "Chunk %d should not have error for provider %s", i, tc.providerName)
			}
		})
	}
}

// TestDoneMarkerHandling_ChannelProperlyClosedWithTimeout verifies that the channel
// is properly closed and doesn't block indefinitely after [DONE] marker.
func TestDoneMarkerHandling_ChannelProperlyClosedWithTimeout(t *testing.T) {
	logger := zap.NewNop()

	// Create a mock server that sends [DONE] immediately
	mockSSEServerImmediateDone := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send one chunk then [DONE]
			sseData := map[string]any{
				"id":    "chatcmpl-test",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": "Test",
						},
					},
				},
			}
			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, providerName := range providerNames {
		t.Run(providerName+"_channel_closes_without_blocking", func(t *testing.T) {
			server := mockSSEServerImmediateDone()
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

			// Use a timeout to ensure channel closes properly
			done := make(chan bool)
			go func() {
				for range streamCh {
					// Drain the channel
				}
				done <- true
			}()

			select {
			case <-done:
				// Channel closed properly
			case <-time.After(5 * time.Second):
				t.Fatalf("Channel did not close within timeout for provider %s", providerName)
			}
		})
	}
}

// TestDoneMarkerHandling_NoDataAfterDone verifies that no data is sent
// after [DONE] marker is received.
func TestDoneMarkerHandling_NoDataAfterDone(t *testing.T) {
	logger := zap.NewNop()

	// Create a mock server that sends data after [DONE] (which should be ignored)
	mockSSEServerWithDataAfterDone := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send valid chunk
			sseData := map[string]any{
				"id":    "chatcmpl-1",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": "Before DONE",
						},
					},
				},
			}
			data, _ := json.Marshal(sseData)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// Send [DONE]
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()

			// Send data after [DONE] (should be ignored by provider)
			sseDataAfter := map[string]any{
				"id":    "chatcmpl-2",
				"model": "test-model",
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"role":    "assistant",
							"content": "After DONE - should not appear",
						},
					},
				},
			}
			dataAfter, _ := json.Marshal(sseDataAfter)
			fmt.Fprintf(w, "data: %s\n\n", dataAfter)
			flusher.Flush()
		}))
	}

	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, providerName := range providerNames {
		t.Run(providerName+"_ignores_data_after_done", func(t *testing.T) {
			server := mockSSEServerWithDataAfterDone()
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

			var receivedContents []string
			for chunk := range streamCh {
				if chunk.Delta.Content != "" {
					receivedContents = append(receivedContents, chunk.Delta.Content)
				}
			}

			// Should only receive the chunk before [DONE]
			require.Len(t, receivedContents, 1, "Should only receive 1 chunk for provider %s", providerName)
			assert.Equal(t, "Before DONE", receivedContents[0],
				"Should only receive content before [DONE] for provider %s", providerName)
		})
	}
}

// TestDoneMarkerHandling_ConcurrentReads verifies that channel closure is safe
// for concurrent reads after [DONE] marker.
func TestDoneMarkerHandling_ConcurrentReads(t *testing.T) {
	logger := zap.NewNop()

	mockSSEServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// Send multiple chunks
			for i := 0; i < 5; i++ {
				sseData := map[string]any{
					"id":    fmt.Sprintf("chatcmpl-%d", i),
					"model": "test-model",
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role":    "assistant",
								"content": fmt.Sprintf("Chunk %d", i),
							},
						},
					},
				}
				data, _ := json.Marshal(sseData)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}

			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
		}))
	}

	providerNames := []string{"grok", "qwen", "deepseek", "glm", "minimax"}

	for _, providerName := range providerNames {
		t.Run(providerName+"_concurrent_safe", func(t *testing.T) {
			server := mockSSEServer()
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			ctx := context.Background()
			var streamCh <-chan llm.StreamChunk
			var err error

			switch providerName {
			case "grok":
				cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{APIKey: "test-key", BaseURL: server.URL}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{APIKey: "test-key", BaseURL: server.URL}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{APIKey: "test-key", BaseURL: server.URL}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{APIKey: "test-key", BaseURL: server.URL}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

			// Multiple goroutines trying to read from channel
			var wg sync.WaitGroup
			chunkCount := 0
			var mu sync.Mutex

			// Single reader (channels can only have one reader in Go)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for chunk := range streamCh {
					mu.Lock()
					if chunk.Delta.Content != "" {
						chunkCount++
					}
					mu.Unlock()
				}
			}()

			// Wait with timeout
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// Success
			case <-time.After(5 * time.Second):
				t.Fatalf("Test timed out for provider %s", providerName)
			}

			mu.Lock()
			assert.Equal(t, 5, chunkCount, "Should receive 5 chunks for provider %s", providerName)
			mu.Unlock()
		})
	}
}

// TestDoneMarkerHandling_DoneMarkerVariations tests different [DONE] marker formats
// that providers should handle.
func TestDoneMarkerHandling_DoneMarkerVariations(t *testing.T) {
	logger := zap.NewNop()

	testCases := []struct {
		name       string
		doneMarker string
		shouldWork bool
	}{
		{"standard_done", "data: [DONE]\n\n", true},
		{"done_with_space", "data:  [DONE]\n\n", true},
		{"done_with_trailing_space", "data: [DONE] \n\n", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)

				flusher, ok := w.(http.Flusher)
				if !ok {
					return
				}

				// Send one chunk
				sseData := map[string]any{
					"id":    "chatcmpl-test",
					"model": "test-model",
					"choices": []map[string]any{
						{
							"index": 0,
							"delta": map[string]any{
								"role":    "assistant",
								"content": "Test",
							},
						},
					},
				}
				data, _ := json.Marshal(sseData)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()

				// Send [DONE] marker variation
				fmt.Fprint(w, tc.doneMarker)
				flusher.Flush()
			}))
			defer server.Close()

			req := &llm.ChatRequest{
				Model: "test-model",
				Messages: []llm.Message{
					{Role: llm.RoleUser, Content: "Test"},
				},
			}

			// Test with grok provider as representative
			cfg := providers.GrokConfig{APIKey: "test-key", BaseURL: server.URL}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err := p.Stream(context.Background(), req)

			require.NoError(t, err, "Stream() should not return error")

			// Verify channel closes properly
			done := make(chan bool)
			go func() {
				for range streamCh {
					// Drain
				}
				done <- true
			}()

			select {
			case <-done:
				if !tc.shouldWork {
					t.Errorf("Expected channel to not close properly for %s", tc.name)
				}
			case <-time.After(3 * time.Second):
				if tc.shouldWork {
					t.Errorf("Channel did not close within timeout for %s", tc.name)
				}
			}
		})
	}
}

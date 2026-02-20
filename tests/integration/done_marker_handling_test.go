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

// 功能：多提供商支持，任务 12.5：[完成] 标记处理
// **验证：要求 10.4**
//
// This 单元测试验证当提供者在 SSE 流中收到“[DONE]”标记时，
// 流通道已正确关闭。

// TestDoneMarkerHandling_StreamClosesOnDone 验证流通道
// 当收到所有 5 个提供商的 [DONE] 标记时关闭。
func TestDoneMarkerHandling_StreamClosesOnDone(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个发送 SSE 数据的模拟服务器，然后发送 [DONE]
	mockSSEServerWithDone := func(chunks []string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 将每个块作为 SSE 数据发送
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

			// 发送 [DONE] 标记以表示流结束
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
				cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error for provider %s", tc.providerName)

			// 收集所有块并验证通道关闭
			var receivedChunks []llm.StreamChunk
			channelClosed := false

			for chunk := range streamCh {
				receivedChunks = append(receivedChunks, chunk)
			}
			channelClosed = true

			// 验证通道在 [DONE] 之后关闭
			assert.True(t, channelClosed, "Channel should be closed after [DONE] for provider %s", tc.providerName)

			// 验证我们收到了预期数量的块（[DONE] 之后没有额外的块）
			assert.Equal(t, len(tc.chunks), len(receivedChunks),
				"Should receive exactly %d chunks before [DONE] for provider %s", len(tc.chunks), tc.providerName)

			// 验证接收到的块中没有错误
			for i, chunk := range receivedChunks {
				assert.Nil(t, chunk.Err, "Chunk %d should not have error for provider %s", i, tc.providerName)
			}
		})
	}
}

// TestDoneMarkerHandling_ChannelProperlyClosedWithTimeout 验证通道
// 已正确关闭，并且不会在 [DONE] 标记之后无限期地阻塞。
func TestDoneMarkerHandling_ChannelProperlyClosedWithTimeout(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个立即发送 [DONE] 的模拟服务器
	mockSSEServerImmediateDone := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 发送一大块然后[完成]
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
				cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

			// 使用超时确保通道正确关闭
			done := make(chan bool)
			go func() {
				for range streamCh {
					// 排空通道
				}
				done <- true
			}()

			select {
			case <-done:
				// 通道正常关闭
			case <-time.After(5 * time.Second):
				t.Fatalf("Channel did not close within timeout for provider %s", providerName)
			}
		})
	}
}

// TestDoneMarkerHandling_NoDataAfterDone 验证没有发送数据
// 收到 [DONE] 标记后。
func TestDoneMarkerHandling_NoDataAfterDone(t *testing.T) {
	logger := zap.NewNop()

	// 创建一个模拟服务器，在 [DONE] 之后发送数据（应该被忽略）
	mockSSEServerWithDataAfterDone := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)

			flusher, ok := w.(http.Flusher)
			if !ok {
				return
			}

			// 发送有效块
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

			// 发送[完成]
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()

			// [DONE]后发送数据（应被提供商忽略）
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
				cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
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

			// 应该只在 [DONE] 之前接收块
			require.Len(t, receivedContents, 1, "Should only receive 1 chunk for provider %s", providerName)
			assert.Equal(t, "Before DONE", receivedContents[0],
				"Should only receive content before [DONE] for provider %s", providerName)
		})
	}
}

// TestDoneMarkerHandling_ConcurrentReads 验证通道关闭是否安全
// 用于 [DONE] 标记之后的并发读取。
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

			// 发送多个块
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
				cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := grok.NewGrokProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "qwen":
				cfg := providers.QwenConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := qwen.NewQwenProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "deepseek":
				cfg := providers.DeepSeekConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := deepseek.NewDeepSeekProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "glm":
				cfg := providers.GLMConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := glm.NewGLMProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			case "minimax":
				cfg := providers.MiniMaxConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
				p := minimax.NewMiniMaxProvider(cfg, logger)
				streamCh, err = p.Stream(ctx, req)
			}

			require.NoError(t, err, "Stream() should not return error for provider %s", providerName)

			// 多个 goroutine 尝试从通道读取
			var wg sync.WaitGroup
			chunkCount := 0
			var mu sync.Mutex

			// 单个阅读器（Go 中通道只能有一个阅读器）
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

			// 等待超时
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				// 成功
			case <-time.After(5 * time.Second):
				t.Fatalf("Test timed out for provider %s", providerName)
			}

			mu.Lock()
			assert.Equal(t, 5, chunkCount, "Should receive 5 chunks for provider %s", providerName)
			mu.Unlock()
		})
	}
}

// TestDoneMarkerHandling_DoneMarkerVariations 测试不同的 [DONE] 标记格式
// 提供商应该处理的。
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

				// 发送一大块
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

				// 发送 [DONE] 标记变体
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

			// 以grok提供商为代表进行测试
			cfg := providers.GrokConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: server.URL}}
			p := grok.NewGrokProvider(cfg, logger)
			streamCh, err := p.Stream(context.Background(), req)

			require.NoError(t, err, "Stream() should not return error")

			// 验证通道是否正确关闭
			done := make(chan bool)
			go func() {
				for range streamCh {
					// 流走
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

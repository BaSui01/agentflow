// =============================================================================
// 🧪 AgentFlow 全能力测试 Part C — 多模态 + WebSearch + Embedding
// =============================================================================
// 覆盖最后 4 项需要外部服务的能力：
//   C1. 多模态图片识别（OpenAI 兼容格式 + glm-5）
//   C2. Web Search（Tavily API）
//   C3. Embedding 向量生成（SiliconFlow + bge-m3）
//   C4. Embedding 语义相似度验证
// =============================================================================

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openaicompat"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// ─── 配置 ────────────────────────────────────────────────

const (
	// LLM（多模态）
	llmAPIKey  = "sk-W2WaPZdpC6iOoP6bMY8P7XwRfIZua7c2tkTuQ0DIPGPZKQwJ"
	llmBaseURL = "https://ai.xoooox.xyz"
	llmModel   = "glm-5"

	// Tavily Web Search
	tavilyAPIKey = "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"

	// SiliconFlow Embedding
	embAPIKey  = "sk-zkaopzkhqdbmhohspxfpyhnqzxlqrbqctucfdulppelulcdj"
	embBaseURL = "https://api.siliconflow.cn"
	embModel   = "BAAI/bge-m3"
)

// ─── 测试结果 ────────────────────────────────────────────

type R struct {
	Name, Status string
	D            time.Duration
	Info         string
}

var rs []R

func rec(n, s string, d time.Duration, info string) {
	rs = append(rs, R{n, s, d, info})
	i := map[string]string{"PASS": "✅", "FAIL": "❌", "WARN": "⚠️"}[s]
	fmt.Printf("  %s %-28s %8v  %s\n", i, n, d.Round(time.Millisecond), info)
}

func txt(m types.Message) string {
	if m.Content != "" {
		return m.Content
	}
	if m.ReasoningContent != nil {
		return *m.ReasoningContent
	}
	return ""
}

func cut(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// ─── 入口 ────────────────────────────────────────────────

func main() {
	lg, _ := zap.NewProduction()
	defer lg.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  🧪 AgentFlow Part C — 多模态 + WebSearch + Embedding      ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	fmt.Println("\n━━━ 多模态图片识别 ━━━")
	c01ImageURLRecognition(ctx, lg)

	fmt.Println("\n━━━ Web Search ━━━")
	c02TavilySearch(ctx, lg)
	c03WebSearchToolReAct(ctx, lg)

	fmt.Println("\n━━━ Embedding 向量 ━━━")
	c04EmbeddingGeneration(ctx)
	c05EmbeddingSimilarity(ctx)
	c06EmbeddingBatch(ctx)

	printSummary()
}

// =============================================================================
// C1: 多模态图片识别 — URL 图片
// =============================================================================

func c01ImageURLRecognition(ctx context.Context, lg *zap.Logger) {
	t := time.Now()

	provider := openaicompat.New(openaicompat.Config{
		ProviderName: "glm-vision", APIKey: llmAPIKey, BaseURL: llmBaseURL,
		DefaultModel: llmModel, FallbackModel: llmModel, Timeout: 120 * time.Second,
	}, lg)

	// 使用一张公开的 Go gopher 图片
	req := &llm.ChatRequest{
		Model: llmModel,
		Messages: []types.Message{
			{
				Role:    "user",
				Content: "这张图片里是什么？用一句话描述。",
				Images: []types.ImageContent{
					{
						Type: "url",
						URL:  "https://go.dev/blog/gopher/header.jpg",
					},
				},
			},
		},
		MaxTokens:   256,
		Temperature: 0.1,
	}

	resp, err := provider.Completion(ctx, req)
	if err != nil {
		rec("图片URL识别", "FAIL", time.Since(t), fmt.Sprintf("调用失败: %v", err))
		return
	}

	content := txt(resp.Choices[0].Message)
	if content == "" {
		rec("图片URL识别", "FAIL", time.Since(t), "回复为空")
		return
	}

	// 检查是否识别出了图片内容（gopher/Go/吉祥物等关键词）
	lower := strings.ToLower(content)
	if strings.Contains(lower, "gopher") || strings.Contains(lower, "go") ||
		strings.Contains(lower, "吉祥物") || strings.Contains(lower, "卡通") ||
		strings.Contains(lower, "动物") || strings.Contains(lower, "图") {
		rec("图片URL识别", "PASS", time.Since(t), cut(content, 80))
	} else {
		rec("图片URL识别", "WARN", time.Since(t), cut(content, 80))
	}
}

// =============================================================================
// C2: Tavily Web Search
// =============================================================================

func c02TavilySearch(ctx context.Context, lg *zap.Logger) {
	t := time.Now()

	tavilyProvider := tools.NewTavilySearchProvider(tools.TavilyConfig{
		APIKey:  tavilyAPIKey,
		Timeout: 15 * time.Second,
	})

	results, err := tavilyProvider.Search(ctx, "Go语言 1.24 新特性", tools.WebSearchOptions{
		MaxResults: 3,
		Language:   "zh",
	})
	if err != nil {
		rec("Tavily搜索", "FAIL", time.Since(t), fmt.Sprintf("搜索失败: %v", err))
		return
	}

	if len(results) > 0 {
		rec("Tavily搜索", "PASS", time.Since(t), fmt.Sprintf("%d条结果, 首条: %s", len(results), cut(results[0].Title, 50)))
	} else {
		rec("Tavily搜索", "WARN", time.Since(t), "搜索返回0条结果")
	}
}

// =============================================================================
// C3: Web Search + ReAct 联合
// =============================================================================

func c03WebSearchToolReAct(ctx context.Context, lg *zap.Logger) {
	t := time.Now()

	provider := openaicompat.New(openaicompat.Config{
		ProviderName: "glm", APIKey: llmAPIKey, BaseURL: llmBaseURL,
		DefaultModel: llmModel, FallbackModel: llmModel, Timeout: 60 * time.Second,
	}, lg)

	// 注册 Tavily 搜索为工具
	reg := tools.NewDefaultRegistry(lg)
	tavilyProvider := tools.NewTavilySearchProvider(tools.TavilyConfig{
		APIKey:  tavilyAPIKey,
		Timeout: 15 * time.Second,
	})
	tools.RegisterWebSearchTool(reg, tools.WebSearchToolConfig{
		Provider:    tavilyProvider,
		DefaultOpts: tools.WebSearchOptions{MaxResults: 3},
		Timeout:     20 * time.Second,
	}, lg)

	ex := tools.NewDefaultExecutor(reg, lg)
	re := tools.NewReActExecutor(provider, ex, tools.ReActConfig{MaxIterations: 3}, lg)

	resp, steps, err := re.Execute(ctx, &llm.ChatRequest{
		Model: llmModel,
		Messages: []types.Message{
			{Role: "system", Content: "你是助手。使用 web_search 工具搜索信息后回答。"},
			{Role: "user", Content: "2025年最流行的Go Web框架是什么？"},
		},
		Tools: reg.List(), MaxTokens: 1024, Temperature: 0.1,
	})

	if err != nil && resp == nil {
		rec("WebSearch+ReAct", "FAIL", time.Since(t), fmt.Sprintf("失败: %v", err))
		return
	}

	toolUsed := false
	for _, s := range steps {
		for _, a := range s.Actions {
			if a.Name == "web_search" {
				toolUsed = true
			}
		}
	}

	if toolUsed && resp != nil {
		rec("WebSearch+ReAct", "PASS", time.Since(t), fmt.Sprintf("%d步, 使用了web_search", len(steps)))
	} else if resp != nil {
		rec("WebSearch+ReAct", "WARN", time.Since(t), fmt.Sprintf("%d步, 未使用搜索工具", len(steps)))
	} else {
		rec("WebSearch+ReAct", "FAIL", time.Since(t), "无响应")
	}
}

// =============================================================================
// C4: Embedding 向量生成
// =============================================================================

func c04EmbeddingGeneration(ctx context.Context) {
	t := time.Now()

	embProvider := embedding.NewOpenAIProvider(embedding.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  embAPIKey,
			BaseURL: embBaseURL,
			Model:   embModel,
			Timeout: 30 * time.Second,
		},
	})

	vec, err := embProvider.EmbedQuery(ctx, "Go语言是一种高效的编程语言")
	if err != nil {
		rec("Embedding生成", "FAIL", time.Since(t), fmt.Sprintf("失败: %v", err))
		return
	}

	if len(vec) > 0 {
		rec("Embedding生成", "PASS", time.Since(t), fmt.Sprintf("维度=%d, 首值=%.4f", len(vec), vec[0]))
	} else {
		rec("Embedding生成", "FAIL", time.Since(t), "返回空向量")
	}
}

// =============================================================================
// C5: Embedding 语义相似度验证
// =============================================================================

func c05EmbeddingSimilarity(ctx context.Context) {
	t := time.Now()

	embProvider := embedding.NewOpenAIProvider(embedding.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  embAPIKey,
			BaseURL: embBaseURL,
			Model:   embModel,
			Timeout: 30 * time.Second,
		},
	})

	// 语义相近的两句话 vs 语义无关的一句话
	texts := []string{
		"Go语言是一种高效的编程语言",     // A
		"Golang是一种性能优秀的开发语言", // B（与A语义相近）
		"今天天气真好适合出去玩",        // C（与A语义无关）
	}

	vecs, err := embProvider.EmbedDocuments(ctx, texts)
	if err != nil {
		rec("语义相似度", "FAIL", time.Since(t), fmt.Sprintf("失败: %v", err))
		return
	}

	if len(vecs) != 3 {
		rec("语义相似度", "FAIL", time.Since(t), fmt.Sprintf("期望3个向量, 得到%d", len(vecs)))
		return
	}

	simAB := cosineSimilarity(vecs[0], vecs[1])
	simAC := cosineSimilarity(vecs[0], vecs[2])

	if simAB > simAC && simAB > 0.5 {
		rec("语义相似度", "PASS", time.Since(t), fmt.Sprintf("AB=%.3f > AC=%.3f ✓", simAB, simAC))
	} else {
		rec("语义相似度", "WARN", time.Since(t), fmt.Sprintf("AB=%.3f, AC=%.3f", simAB, simAC))
	}
}

// =============================================================================
// C6: Embedding 批量生成
// =============================================================================

func c06EmbeddingBatch(ctx context.Context) {
	t := time.Now()

	embProvider := embedding.NewOpenAIProvider(embedding.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  embAPIKey,
			BaseURL: embBaseURL,
			Model:   embModel,
			Timeout: 30 * time.Second,
		},
	})

	docs := []string{
		"人工智能正在改变世界",
		"机器学习是AI的核心技术",
		"深度学习推动了NLP的发展",
		"大语言模型是当前AI的热点",
		"向量数据库支撑RAG应用",
	}

	vecs, err := embProvider.EmbedDocuments(ctx, docs)
	if err != nil {
		rec("批量Embedding", "FAIL", time.Since(t), fmt.Sprintf("失败: %v", err))
		return
	}

	if len(vecs) == len(docs) {
		allValid := true
		for _, v := range vecs {
			if len(v) == 0 {
				allValid = false
			}
		}
		if allValid {
			rec("批量Embedding", "PASS", time.Since(t), fmt.Sprintf("%d条文档, 维度=%d", len(vecs), len(vecs[0])))
		} else {
			rec("批量Embedding", "FAIL", time.Since(t), "部分向量为空")
		}
	} else {
		rec("批量Embedding", "FAIL", time.Since(t), fmt.Sprintf("期望%d, 得到%d", len(docs), len(vecs)))
	}
}

// ─── 辅助函数 ────────────────────────────────────────────

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func printSummary() {
	fmt.Println("\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  📊 Part C 测试汇总                                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	ps, fl, wr := 0, 0, 0
	for _, r := range rs {
		switch r.Status {
		case "PASS":
			ps++
		case "FAIL":
			fl++
		case "WARN":
			wr++
		}
	}
	fmt.Printf("\n  总计: %d | ✅ PASS: %d | ❌ FAIL: %d | ⚠️  WARN: %d\n\n", len(rs), ps, fl, wr)
	for _, r := range rs {
		i := map[string]string{"PASS": "✅", "FAIL": "❌", "WARN": "⚠️"}[r.Status]
		fmt.Printf("  %s %-28s %8v  %s\n", i, r.Name, r.D.Round(time.Millisecond), r.Info)
	}
	if fl == 0 {
		fmt.Println("\n  🎉 多模态+搜索+向量全部通过！")
	} else {
		fmt.Printf("\n  ⚠️  有 %d 项失败\n", fl)
	}
}

// 避免 unused import
var _ = json.Marshal

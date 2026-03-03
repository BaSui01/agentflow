package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/browser"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/voice"
	"github.com/BaSui01/agentflow/pkg/cache"
	"github.com/BaSui01/agentflow/pkg/database"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	fmt.Println("=== 2026 Advanced Features Demo ===")

	// 1. Layered Memory System
	demoLayeredMemory(logger)

	// 2. GraphRAG
	demoGraphRAG(ctx, logger)

	// 3. Agentic Browser (mock)
	demoAgenticBrowser(logger)

	// 4. Native Audio Reasoning (mock)
	demoNativeAudio(logger)

	// 5. Shadow AI Detection
	demoShadowAI(ctx, logger)

	// 6. Infra Managers
	demoInfraManagers(ctx, logger)

	// 7. Types Utilities
	demoTypesUtilities(ctx)

	fmt.Println("\n=== All 2026 Features Demo Complete ===")
}

func demoLayeredMemory(logger *zap.Logger) {
	fmt.Println("--- 1. Layered Memory System ---")

	config := memory.LayeredMemoryConfig{
		EpisodicMaxSize:  100,
		WorkingCapacity:  50,
		WorkingTTL:       10 * time.Minute,
		ProceduralConfig: memory.ProceduralConfig{MaxProcedures: 100},
	}

	lm := memory.NewLayeredMemory(config, logger)

	// Store episodic memory
	lm.Episodic.Store(&memory.Episode{
		Context: "User asked about weather",
		Action:  "Called weather API",
		Result:  "Returned sunny forecast",
	})

	// Store working memory
	lm.Working.Set("current_task", "demo", 10)

	// Store procedural memory
	lm.Procedural.Store(&memory.Procedure{
		Name:     "Weather Query",
		Steps:    []string{"Parse query", "Call API", "Format response"},
		Triggers: []string{"weather", "forecast"},
	})

	fmt.Printf("  Episodic memories: %d\n", len(lm.Episodic.Recall(10)))
	fmt.Printf("  Working items: %d\n", len(lm.Working.GetAll()))
	fmt.Println("  ✓ Layered memory initialized")
}

func demoGraphRAG(ctx context.Context, logger *zap.Logger) {
	fmt.Println("\n--- 2. GraphRAG ---")

	graph := rag.NewKnowledgeGraph(logger)

	// Add nodes
	graph.AddNode(&rag.Node{ID: "doc1", Type: "document", Label: "AI Overview"})
	graph.AddNode(&rag.Node{ID: "entity1", Type: "concept", Label: "Machine Learning"})

	// Add edge
	graph.AddEdge(&rag.Edge{Source: "doc1", Target: "entity1", Type: "mentions"})

	// Query neighbors
	neighbors := graph.GetNeighbors("doc1", 2)
	fmt.Printf("  Graph nodes: 2, Neighbors of doc1: %d\n", len(neighbors))
	fmt.Println("  ✓ GraphRAG knowledge graph ready")
}

func demoAgenticBrowser(logger *zap.Logger) {
	fmt.Println("\n--- 3. Agentic Browser ---")

	config := browser.DefaultBrowserConfig()
	fmt.Printf("  Headless: %v\n", config.Headless)
	fmt.Printf("  Viewport: %dx%d\n", config.ViewportWidth, config.ViewportHeight)
	fmt.Printf("  Timeout: %v\n", config.Timeout)

	tool := browser.NewBrowserTool(browser.NewChromeDPBrowserFactory(logger), config, logger)
	_, _ = tool.ExecuteCommand(context.Background(), "demo-session", browser.BrowserCommand{
		Action: browser.ActionNavigate,
		Value:  "https://example.com",
	})
	_ = tool.CloseSession("demo-session")
	_ = tool.CloseAll()

	fmt.Println("  ✓ Browser tool path wired")
}

func demoNativeAudio(logger *zap.Logger) {
	fmt.Println("\n--- 4. Native Audio Reasoning ---")

	config := voice.DefaultNativeAudioConfig()
	fmt.Printf("  Target latency: %dms\n", config.TargetLatencyMS)
	fmt.Printf("  Sample rate: %d Hz\n", config.SampleRate)
	fmt.Printf("  VAD enabled: %v\n", config.EnableVAD)
	fmt.Println("  ✓ Native audio config ready (requires provider)")
}

func demoShadowAI(ctx context.Context, logger *zap.Logger) {
	fmt.Println("\n--- 5. Shadow AI Detection ---")

	config := guardrails.DefaultShadowAIConfig()
	detector := guardrails.NewShadowAIDetector(config, logger)

	// Test domain detection
	detection := detector.CheckDomain("api.openai.com")
	if detection != nil {
		fmt.Printf("  Detected: %s (severity: %s)\n", detection.PatternName, detection.Severity)
	}

	// Test content scan
	testContent := `config.api_key = "sk-1234567890abcdef1234567890abcdef1234567890abcdef"`
	detections := detector.ScanContent(ctx, testContent, "config.yaml", "user1")
	fmt.Printf("  Content scan detections: %d\n", len(detections))

	stats := detector.GetStats()
	fmt.Printf("  Total detections: %d\n", stats.TotalDetections)
	fmt.Println("  ✓ Shadow AI detector operational")
}

func demoInfraManagers(ctx context.Context, logger *zap.Logger) {
	fmt.Println("\n--- 6. Infra Managers ---")

	cacheCfg := cache.DefaultConfig()
	cacheCfg.HealthCheckInterval = 0
	cacheManager, err := cache.NewManager(cacheCfg, logger)
	if err != nil {
		fmt.Printf("  Cache manager unavailable (skip): %v\n", err)
	} else {
		defer cacheManager.Close()

		_ = cacheManager.Set(ctx, "demo:plain", "value", time.Minute)
		_, _ = cacheManager.Get(ctx, "demo:plain")

		payload := map[string]any{"feature": "infra", "year": 2026}
		_ = cacheManager.SetJSON(ctx, "demo:json", payload, time.Minute)
		var out map[string]any
		_ = cacheManager.GetJSON(ctx, "demo:json", &out)
		_, _ = json.Marshal(out)

		_ = cacheManager.Delete(ctx, "demo:plain", "demo:json")
		stats, statsErr := cacheManager.GetStats(ctx)
		if statsErr != nil {
			fmt.Printf("  Cache stats unavailable: %v\n", statsErr)
		} else {
			fmt.Printf("  Cache stats: keys=%d, hits=%d, misses=%d\n", stats.Keys, stats.Hits, stats.Misses)
		}
		fmt.Println("  ✓ Cache manager path wired")
	}

	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		fmt.Printf("  DB open failed: %v\n", err)
		return
	}
	poolManager, err := database.NewPoolManager(gdb, database.DefaultPoolConfig(), logger)
	if err != nil {
		fmt.Printf("  Pool manager init failed: %v\n", err)
		return
	}
	defer poolManager.Close()

	poolStats := poolManager.GetStats()
	fmt.Printf("  Pool stats: open=%d, in_use=%d, idle=%d\n", poolStats.OpenConnections, poolStats.InUse, poolStats.Idle)
	fmt.Println("  ✓ Database pool manager path wired")
}

func demoTypesUtilities(ctx context.Context) {
	fmt.Println("\n--- 7. Types Utilities ---")

	ctx = types.WithTraceID(ctx, "trace-2026")
	ctx = types.WithTenantID(ctx, "tenant-demo")
	ctx = types.WithUserID(ctx, "user-demo")
	ctx = types.WithRunID(ctx, "run-demo")
	ctx = types.WithParentRunID(ctx, "run-parent")
	ctx = types.WithSpanID(ctx, "span-demo")
	ctx = types.WithAgentID(ctx, "agent-demo")
	ctx = types.WithLLMModel(ctx, "gpt-4o-mini")
	ctx = types.WithPromptBundleVersion(ctx, "v1")
	ctx = types.WithRoles(ctx, []string{"admin", "reviewer"})

	_, _ = types.UserID(ctx)
	_, _ = types.ParentRunID(ctx)
	_, _ = types.AgentID(ctx)
	_, _ = types.Roles(ctx)

	_ = types.NewMessage(types.RoleUser, "hello")
	_ = types.NewSystemMessage("system")
	_ = types.NewUserMessage("user")
	_ = types.NewAssistantMessage("assistant")
	_ = types.NewToolMessage("call-1", "tool-a", "ok")
	_ = types.NewDeveloperMessage("developer")

	schema := types.NewObjectSchema().
		AddProperty("name", types.NewStringSchema()).
		AddProperty("age", types.NewIntegerSchema()).
		AddProperty("active", types.NewBooleanSchema()).
		AddProperty("score", types.NewNumberSchema()).
		AddProperty("tags", types.NewArraySchema(types.NewStringSchema())).
		AddProperty("level", types.NewEnumSchema("L1", "L2", "L3")).
		AddRequired("name", "age").
		WithDescription("demo schema")
	raw, _ := schema.ToJSON()
	_, _ = types.FromJSON(raw)

	_ = types.DefaultReflectionConfig()
	_ = types.DefaultToolSelectionConfig()
	_ = types.DefaultPromptEnhancerConfig()
	_ = types.DefaultGuardrailsConfig()
	_ = types.DefaultMemoryConfig()
	_ = types.DefaultObservabilityConfig()

	timeoutErr := types.NewTimeoutError("timeout")
	_ = types.IsRetryable(timeoutErr)
	_ = types.GetErrorCode(timeoutErr)
	wrapped := types.WrapErrorf(errors.New("boom"), types.ErrInternalError, "wrapped %s", "error")
	_, _ = types.AsError(wrapped)
	_ = types.IsErrorCode(wrapped, types.ErrInternalError)
	_ = types.NewAuthenticationError("auth failed")

	fmt.Println("  ✓ Types utility paths wired")
}

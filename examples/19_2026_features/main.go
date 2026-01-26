// Package main demonstrates 2026 advanced features.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/browser"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/voice"
	"github.com/BaSui01/agentflow/rag"
	"go.uber.org/zap"
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

	config := browser.DefaultAgenticBrowserConfig()
	fmt.Printf("  Max actions: %d\n", config.MaxActions)
	fmt.Printf("  Action delay: %v\n", config.ActionDelay)
	fmt.Printf("  Timeout: %v\n", config.Timeout)
	fmt.Println("  ✓ Agentic browser config ready (requires driver)")
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

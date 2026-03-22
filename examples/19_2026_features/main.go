package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/evaluation"
	"github.com/BaSui01/agentflow/agent/execution"
	"github.com/BaSui01/agentflow/agent/guardrails"
	"github.com/BaSui01/agentflow/agent/memory"
	"github.com/BaSui01/agentflow/agent/memorycore"
	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/voice"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/pkg/cache"
	"github.com/BaSui01/agentflow/pkg/database"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/types"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
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

	// 3. Native Audio Reasoning (mock)
	demoNativeAudio(logger)

	// 4. Shadow AI Detection
	demoShadowAI(ctx, logger)

	// 5. Infra Managers
	demoInfraManagers(ctx, logger)

	// 6. Types Utilities
	demoTypesUtilities(ctx)

	// 7. Discovery Subsystem
	demoDiscoverySubsystem(ctx, logger)

	// 8. Evaluation Subsystem
	demoEvaluationSubsystem(ctx, logger)

	// 9. Execution Subsystem
	demoExecutionSubsystem(ctx, logger)

	fmt.Println("\n=== All 2026 Features Demo Complete ===")
}

func demoLayeredMemory(logger *zap.Logger) {
	ctx := context.Background()
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
	proc := &memory.Procedure{
		Name:     "Weather Query",
		Steps:    []string{"Parse query", "Call API", "Format response"},
		Triggers: []string{"weather", "forecast"},
	}
	lm.Procedural.Store(proc)

	fmt.Printf("  Episodic memories: %d\n", len(lm.Episodic.Recall(10)))
	fmt.Printf("  Episodic search hits: %d\n", len(lm.Episodic.Search("weather", 5)))
	_, _ = lm.Working.Get("current_task")
	lm.Working.Clear()
	fmt.Printf("  Working items: %d\n", len(lm.Working.GetAll()))
	_ = lm.Semantic.StoreFact(ctx, &memory.Fact{
		Subject:    "agentflow",
		Predicate:  "supports",
		Object:     "layered_memory",
		Confidence: 0.9,
	})
	semanticFacts := lm.Semantic.Query("agentflow")
	if len(semanticFacts) > 0 {
		_, _ = lm.Semantic.GetFact(semanticFacts[0].ID)
	}
	if proc.ID != "" {
		_, _ = lm.Procedural.Get(proc.ID)
	}
	_ = lm.Procedural.FindByTrigger("weather")
	_, _ = lm.Export()

	episodicStore := memory.NewInMemoryEpisodicStore(logger)
	_ = episodicStore.RecordEvent(ctx, &types.EpisodicEvent{
		AgentID: "demo-agent",
		Type:    "query",
		Content: "user asked weather",
		Context: map[string]any{"city": "Shanghai"},
	})
	_, _ = episodicStore.QueryEvents(ctx, memory.EpisodicQuery{
		AgentID: "demo-agent",
		Type:    "query",
		Limit:   10,
	})
	_, _ = episodicStore.GetTimeline(ctx, "demo-agent", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))

	decayCfg := memory.DefaultDecayConfig()
	decayCfg.DecayInterval = 10 * time.Millisecond
	decayCfg.HalfLife = 30 * time.Second
	decayCfg.MaxMemories = 2
	decay := memory.NewIntelligentDecay(decayCfg, logger)
	item1 := &memory.MemoryItem{
		ID:        "m1",
		Content:   "weather preference: sunny",
		Vector:    []float64{1, 0, 0},
		Relevance: 0.9,
	}
	item2 := &memory.MemoryItem{
		ID:        "m2",
		Content:   "travel preference: beach",
		Vector:    []float64{0.9, 0.1, 0},
		Relevance: 0.7,
	}
	decay.Add(item1)
	decay.Add(item2)
	_ = item1.CompositeScore(decayCfg)
	_ = item1.RecencyScore(decayCfg.HalfLife)
	_ = decay.Get("m1")
	_ = decay.Search([]float64{1, 0, 0}, 2)
	_ = decay.Decay(ctx)
	_ = decay.UpdateRelevance("m2", 0.95)
	_ = decay.GetStats()
	decayCtx, decayCancel := context.WithCancel(ctx)
	_ = decay.Start(decayCtx)
	time.Sleep(20 * time.Millisecond)
	decay.Stop()
	decayCancel()

	kg := memory.NewInMemoryKnowledgeGraph(logger)
	_ = kg.AddEntity(ctx, &memory.Entity{ID: "agentflow", Type: "project", Name: "AgentFlow"})
	_ = kg.AddEntity(ctx, &memory.Entity{ID: "memory", Type: "feature", Name: "Memory"})
	_ = kg.AddEntity(ctx, &memory.Entity{ID: "rag", Type: "feature", Name: "RAG"})
	_ = kg.AddRelation(ctx, &memory.Relation{FromID: "agentflow", ToID: "memory", Type: "supports", Weight: 1})
	_ = kg.AddRelation(ctx, &memory.Relation{FromID: "agentflow", ToID: "rag", Type: "supports", Weight: 1})
	_, _ = kg.QueryEntity(ctx, "agentflow")
	_, _ = kg.QueryRelations(ctx, "agentflow", "supports")
	_, _ = kg.FindPath(ctx, "memory", "rag", 4)

	memoryMgr := newDemoMemoryManager()
	coordinator := memorycore.NewCoordinator("demo-agent", memoryMgr, logger)
	_ = coordinator.Save(ctx, "user asked weather", memorycore.MemoryShortTerm, map[string]any{"channel": "chat"})
	_ = coordinator.LoadRecent(ctx, memorycore.MemoryShortTerm, 10)
	_, _ = coordinator.Search(ctx, "weather", 5)
	_ = coordinator.GetRecentMemory()
	coordinator.ClearRecentMemory()
	_ = coordinator.GetMemoryManager()
	_ = coordinator.SaveConversation(ctx, "hello", "hi there")
	_, _ = coordinator.RecallRelevant(ctx, "hello", 3)

	ns := memorycore.NewNamespacedManager(memoryMgr, "subagent-1")
	_ = ns.Namespace()
	_ = ns.Save(ctx, memorycore.MemoryRecord{
		ID:      "ns-1",
		AgentID: "worker-a",
		Kind:    memorycore.MemoryShortTerm,
		Content: "namespaced memory item",
	})
	_, _ = ns.LoadRecent(ctx, "worker-a", memorycore.MemoryShortTerm, 5)
	_, _ = ns.Search(ctx, "worker-a", "memory", 5)
	_, _ = ns.Get(ctx, "ns-1")
	_ = ns.Delete(ctx, "ns-1")
	_ = ns.Clear(ctx, "worker-a", memorycore.MemoryShortTerm)

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

func demoNativeAudio(logger *zap.Logger) {
	fmt.Println("\n--- 3. Native Audio Reasoning ---")

	config := voice.DefaultNativeAudioConfig()
	fmt.Printf("  Target latency: %dms\n", config.TargetLatencyMS)
	fmt.Printf("  Sample rate: %d Hz\n", config.SampleRate)
	fmt.Printf("  VAD enabled: %v\n", config.EnableVAD)
	fmt.Println("  ✓ Native audio config ready (requires provider)")
}

func demoShadowAI(ctx context.Context, logger *zap.Logger) {
	fmt.Println("\n--- 4. Shadow AI Detection ---")

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
	fmt.Println("\n--- 5. Infra Managers ---")

	cacheCfg := cache.DefaultConfig()
	cacheCfg.HealthCheckInterval = 0
	if isCacheBackendReachable(cacheCfg.Addr) {
		cacheManager, err := cache.NewManager(cacheCfg, logger)
		if err != nil {
			fmt.Println("  Cache manager demo: backend detected but initialization not completed")
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
	} else {
		fmt.Println("  Cache manager demo: no cache backend listening on configured address")
	}

	gdb, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		fmt.Printf("  Database demo unavailable: %v\n", err)
		return
	}
	poolManager, err := database.NewPoolManager(gdb, database.DefaultPoolConfig(), logger)
	if err != nil {
		fmt.Printf("  Pool manager demo unavailable: %v\n", err)
		return
	}
	defer poolManager.Close()

	poolStats := poolManager.GetStats()
	fmt.Printf("  Pool stats: open=%d, in_use=%d, idle=%d\n", poolStats.OpenConnections, poolStats.InUse, poolStats.Idle)
	fmt.Println("  ✓ Database pool manager path wired")
}

func isCacheBackendReachable(addr string) bool {
	if strings.TrimSpace(addr) == "" {
		return false
	}

	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func demoTypesUtilities(ctx context.Context) {
	fmt.Println("\n--- 6. Types Utilities ---")

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

func demoDiscoverySubsystem(ctx context.Context, logger *zap.Logger) {
	fmt.Println("\n--- 7. Discovery Subsystem ---")

	port := findFreePort()
	serviceCfg := discovery.DefaultServiceConfig()
	serviceCfg.EnableAutoRegistration = false
	serviceCfg.HeartbeatInterval = 50 * time.Millisecond
	serviceCfg.Protocol.EnableHTTP = true
	serviceCfg.Protocol.HTTPHost = "127.0.0.1"
	serviceCfg.Protocol.HTTPPort = port
	serviceCfg.Protocol.EnableMulticast = false
	serviceCfg.Protocol.DiscoveryTimeout = 2 * time.Second

	service := discovery.NewDiscoveryService(serviceCfg, logger)
	_ = service.Start(ctx)
	defer service.Stop(ctx)

	card := discovery.CreateAgentCard(
		"demo-agent",
		"discovery demo agent",
		fmt.Sprintf("http://127.0.0.1:%d", port),
		"1.0.0",
		[]a2a.Capability{
			{Name: "search", Description: "search documents", Type: a2a.CapabilityTypeQuery},
			{Name: "summarize", Description: "summarize text", Type: a2a.CapabilityTypeTask},
		},
	)
	info := discovery.AgentInfoFromCard(card, true)
	if info != nil {
		info.Load = 0.2
		info.Priority = 5
		_ = service.RegisterAgent(ctx, info)
	}

	service.RegisterDependency("summarize", []string{"search"})
	service.RegisterExclusiveGroup([]string{"gpu_train", "cpu_train"})

	_, _ = service.FindAgent(ctx, "search docs", []string{"search"})
	_, _ = service.FindAgents(ctx, &discovery.MatchRequest{
		TaskDescription:      "summarize docs",
		RequiredCapabilities: []string{"summarize"},
		Strategy:             discovery.MatchStrategyBestMatch,
		Limit:                5,
	})
	comp, _ := service.ComposeCapabilities(ctx, &discovery.CompositionRequest{
		TaskDescription:      "search and summarize",
		RequiredCapabilities: []string{"search", "summarize"},
		AllowPartial:         true,
		MaxAgents:            2,
	})
	_, _ = service.DiscoverAgents(ctx, &discovery.DiscoveryFilter{Capabilities: []string{"search"}})
	_, _ = service.GetAgent(ctx, "demo-agent")
	_, _ = service.ListAgents(ctx)
	_, _ = service.GetCapability(ctx, "demo-agent", "search")
	_, _ = service.FindCapabilities(ctx, "search")
	_ = service.RecordExecution(ctx, "demo-agent", "search", true, 30*time.Millisecond)
	subID := service.Subscribe(func(event *discovery.DiscoveryEvent) {})
	service.Unsubscribe(subID)
	annSubID := service.SubscribeToAnnouncements(func(agent *discovery.AgentInfo) {})
	service.UnsubscribeFromAnnouncements(annSubID)
	_ = service.RegisterLocalAgent(info)
	_ = service.UpdateLocalAgentLoad(0.3)
	_, _ = service.Registry(), service.Matcher()
	_, _ = service.Composer(), service.Protocol()

	// Registry/store helpers.
	memStore := discovery.NewInMemoryRegistryStore()
	_ = memStore.Save(ctx, info)
	_, _ = memStore.Load(ctx, "demo-agent")
	_, _ = memStore.LoadAll(ctx)
	_ = memStore.Delete(ctx, "demo-agent")

	regWithStore := discovery.NewCapabilityRegistry(discovery.DefaultRegistryConfig(), logger, discovery.WithStore(memStore))
	_ = regWithStore.Start(ctx)
	_ = regWithStore.Close()

	// Matcher/composer direct helpers.
	if matcher, ok := service.Matcher().(*discovery.CapabilityMatcher); ok {
		_, _ = matcher.GetNextRoundRobin(ctx, "search")
		_, _ = matcher.FindBestAgent(ctx, "search", []string{"search"})
		_, _ = matcher.FindLeastLoadedAgent(ctx, []string{"search"})
	}
	if composer, ok := service.Composer().(*discovery.CapabilityComposer); ok {
		composer.RegisterResourceRequirement(&discovery.ResourceRequirement{
			CapabilityName:     "search",
			ExclusiveResources: []string{"db-main"},
		})
		_, _ = composer.GetDependencies("summarize"), composer.GetExclusiveGroups()
		composer.ClearDependencies()
		composer.ClearExclusiveGroups()
		// re-register for executor flow
		composer.RegisterDependency("summarize", []string{"search"})
	}

	// Protocol remote calls to trigger HTTP handlers.
	if proto, ok := service.Protocol().(*discovery.DiscoveryProtocol); ok {
		baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
		_ = proto.AnnounceRemote(ctx, baseURL, info)
		_, _ = proto.DiscoverRemote(ctx, baseURL, &discovery.DiscoveryFilter{
			Capabilities: []string{"search"},
			Tags:         []string{"demo"},
		})
		resp, _ := http.Get(baseURL + "/discovery/agents/demo-agent")
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		healthResp, _ := http.Get(baseURL + "/discovery/health")
		if healthResp != nil && healthResp.Body != nil {
			healthResp.Body.Close()
		}
	}

	// Integration + composition executor.
	integration := discovery.NewAgentDiscoveryIntegration(service, discovery.DefaultIntegrationConfig(), logger)
	_ = integration.Start(ctx)
	defer integration.Stop(ctx)
	provider := &demoCapabilityProvider{card: card}
	_ = integration.RegisterAgent(ctx, provider)
	integration.SetLoadReporter(provider.ID(), func() float64 { return 0.4 })
	_ = integration.UpdateAgentCapabilities(ctx, provider.ID())
	_ = integration.RecordExecution(ctx, provider.ID(), "search", true, 20*time.Millisecond)
	_, _ = integration.FindAgentForTask(ctx, "search", []string{"search"})
	_, _ = integration.FindAgentsForTask(ctx, &discovery.MatchRequest{RequiredCapabilities: []string{"search"}})
	_, _ = integration.ComposeAgentsForTask(ctx, &discovery.CompositionRequest{
		RequiredCapabilities: []string{"search"},
		AllowPartial:         true,
	})
	_ = integration.GetRegisteredAgents()
	_ = integration.IsAgentRegistered(provider.ID())
	_ = integration.DiscoveryService()
	_ = integration.UnregisterAgent(ctx, provider.ID())

	discovery.InitGlobalDiscoveryService(serviceCfg, logger)
	_ = discovery.GetGlobalDiscoveryService()
	discovery.SetGlobalDiscoveryService(service)

	discovery.InitGlobalIntegration(service, discovery.DefaultIntegrationConfig(), logger)
	_ = discovery.GetGlobalIntegration()
	discovery.SetGlobalIntegration(integration)

	if comp != nil && comp.Complete {
		executor := discovery.NewCompositionExecutor(&demoAgentExecutor{}, logger)
		_, _ = executor.Execute(ctx, comp, map[string]any{"query": "demo"})
	}

	_ = service.UnregisterAgent(ctx, "demo-agent")
	fmt.Println("  ✓ Discovery subsystem paths wired")
}

func demoEvaluationSubsystem(ctx context.Context, logger *zap.Logger) {
	fmt.Println("\n--- 8. Evaluation Subsystem ---")

	in := evaluation.NewEvalInput("summarize service errors").
		WithContext(map[string]any{"scene": "demo"}).
		WithExpected("service errors summarized").
		WithReference("incident notes")
	out := evaluation.NewEvalOutput("service errors summarized").
		WithTokensUsed(120).
		WithLatency(300 * time.Millisecond).
		WithCost(0.02).
		WithMetadata(map[string]any{"source": "demo"}).
		WithRetrievedDocIDs([]string{"doc1", "doc2", "doc3"}).
		WithAnswerGroundedness(0.88).
		WithRAGEvalMetrics(types.RAGEvalMetrics{RecallAtK: 0.9, MRR: 0.8, Faithfulness: 0.88})
	_ = in.WithRelevantDocIDs([]string{"doc1", "doc3"})

	metricRes := evaluation.NewMetricEvalResult("input-1").
		AddMetric("accuracy", 1.0).
		AddError("none").
		SetPassed(true)
	_ = metricRes

	reg := evaluation.NewMetricRegistry()
	reg.Register(evaluation.NewAccuracyMetric())
	reg.Register(evaluation.NewLatencyMetric())
	reg.Register(evaluation.NewLatencyMetricWithThreshold(500))
	reg.Register(evaluation.NewTokenUsageMetric())
	reg.Register(evaluation.NewTokenUsageMetricWithMax(500))
	reg.Register(evaluation.NewCostMetric())
	reg.Register(evaluation.NewCostMetricWithMax(1.0))
	reg.Register(evaluation.NewRecallAtKMetric())
	reg.Register(evaluation.NewMRRMetric())
	reg.Register(evaluation.NewGroundednessMetric())
	_, _ = reg.Get("accuracy")
	_ = reg.List()
	_, _ = reg.ComputeAll(ctx, in, out)

	builtinReg := evaluation.NewRegistryWithBuiltinMetrics()
	evaluation.RegisterBuiltinMetrics(builtinReg)

	exactScorer := &evaluation.ExactMatchScorer{}
	containsScorer := &evaluation.ContainsScorer{}
	jsonScorer := &evaluation.JSONScorer{}
	task := &evaluation.EvalTask{
		ID:       "task-1",
		Name:     "task",
		Input:    "summarize",
		Expected: "service errors summarized",
		Metadata: map[string]string{"type": "contains"},
	}
	_, _, _ = exactScorer.Score(ctx, task, "service errors summarized")
	_, _, _ = containsScorer.Score(ctx, task, "prefix service errors summarized suffix")
	task.Expected = `{"ok":true}`
	_, _, _ = jsonScorer.Score(ctx, task, `{"ok":true}`)

	evaluatorCfg := evaluation.DefaultEvaluatorConfig()
	evaluatorCfg.AlertThresholds = []evaluation.AlertThreshold{
		{MetricName: "score", Operator: "lt", Value: 0.95, Level: evaluation.AlertLevelWarning, Message: "score below target"},
	}
	evaluator := evaluation.NewEvaluator(evaluatorCfg, logger)
	evaluator.SetMetricRegistry(builtinReg)
	evaluator.AddAlertHandler(func(alert *evaluation.Alert) {})
	evaluator.RegisterScorer("contains", containsScorer)
	suite := &evaluation.EvalSuite{
		ID:      "suite-1",
		Name:    "demo-suite",
		Version: "1.0.0",
		Tasks: []evaluation.EvalTask{
			{ID: "t1", Name: "contains", Input: "hello", Expected: "hello", Metadata: map[string]string{"type": "contains"}},
			{ID: "t2", Name: "exact", Input: "world", Expected: "world", Metadata: map[string]string{"type": "exact"}},
		},
	}
	report, _ := evaluator.Evaluate(ctx, suite, &demoEvalExecutor{})
	_, _ = evaluator.EvaluateBatch(ctx, []*evaluation.EvalSuite{suite}, &demoEvalExecutor{})
	_ = evaluator.GetAlerts()
	evaluator.ClearAlerts()
	_ = evaluator.GenerateReport([]*evaluation.EvalReport{report})

	expStore := evaluation.NewMemoryExperimentStore()
	tester := evaluation.NewABTester(expStore, logger)
	exp := &evaluation.Experiment{
		ID:   "exp-1",
		Name: "prompt ab test",
		Variants: []evaluation.Variant{
			{ID: "control", Name: "control", Weight: 0.5, IsControl: true, Config: map[string]any{"prompt": "base"}},
			{ID: "treatment", Name: "treatment", Weight: 0.5, Config: map[string]any{"prompt": "improved"}},
		},
		Metrics: []string{"accuracy"},
	}
	_ = tester.CreateExperiment(exp)
	_, _ = tester.GetExperiment(exp.ID)
	_ = tester.StartExperiment(exp.ID)
	assigned, _ := tester.Assign(exp.ID, "user-1")
	if assigned != nil {
		_ = tester.RecordResult(exp.ID, assigned.ID, &evaluation.EvalResult{
			TaskID:  "t1",
			Success: true,
			Output:  "ok",
			Score:   0.95,
			Metrics: map[string]float64{"accuracy": 0.95},
		})
	}
	_ = tester.RecordResult(exp.ID, "control", &evaluation.EvalResult{
		TaskID:  "t2",
		Success: true,
		Output:  "ok",
		Score:   0.80,
		Metrics: map[string]float64{"accuracy": 0.80},
	})
	_ = tester.RecordResult(exp.ID, "treatment", &evaluation.EvalResult{
		TaskID:  "t3",
		Success: true,
		Output:  "ok",
		Score:   0.99,
		Metrics: map[string]float64{"accuracy": 0.99},
	})
	_, _ = tester.Analyze(ctx, exp.ID)
	_ = tester.ListExperiments()
	_, _ = tester.GenerateReport(ctx, exp.ID)
	_, _ = tester.AutoSelectWinner(ctx, exp.ID, 0.90)
	_ = tester.PauseExperiment(exp.ID)
	_ = tester.CompleteExperiment(exp.ID)
	_ = tester.DeleteExperiment(exp.ID)

	_, _ = expStore.ListExperiments(ctx)
	_ = expStore.RecordAssignment(ctx, "exp-2", "user-2", "control")
	_, _ = expStore.GetAssignment(ctx, "exp-2", "user-2")
	_ = expStore.RecordResult(ctx, "exp-2", "control", &evaluation.EvalResult{TaskID: "x", Score: 0.5})
	_, _ = expStore.GetResults(ctx, "exp-2")
	_ = expStore.GetAssignmentCount("exp-2")
	_ = expStore.GetResultCount("exp-2")
	_ = expStore.DeleteExperiment(ctx, "exp-2")

	judgeCfg := evaluation.DefaultLLMJudgeConfig()
	judgeCfg.Model = "demo-judge"
	judge := evaluation.NewLLMJudge(&demoJudgeProvider{}, judgeCfg, logger)
	judgeRes, _ := judge.Judge(ctx, in, out)
	batchRes, _ := judge.JudgeBatch(ctx, []evaluation.InputOutputPair{
		{Input: in, Output: out},
		{Input: in, Output: out},
	})
	_ = judge.AggregateResults(append([]*evaluation.JudgeResult{judgeRes}, batchRes...))
	_ = judge.GetConfig()

	researchCfg := evaluation.DefaultResearchEvalConfig()
	researchCfg.PassThreshold = 0.7
	researchEvaluator := evaluation.NewResearchEvaluator(researchCfg, logger)
	evaluation.RegisterResearchMetrics(researchEvaluator, logger)
	researchEvaluator.RegisterMetric(evaluation.DimensionRelevance, evaluation.NewClarityMetric(logger))
	researchEvaluator.RegisterMetric(evaluation.DimensionReproducibility, evaluation.NewRigorMetric(logger))

	researchIn := evaluation.NewEvalInput("Propose a novel method for robust retrieval-augmented generation")
	researchOut := evaluation.NewEvalOutput(`Introduction: We propose a novel and unique approach.
Methodology: The experiment uses baseline comparison, ablation, statistical significance and confidence interval.
Results: Accuracy and recall improved by 12% compared to previous work.
Discussion: However, we document limitation and future work.
Conclusion: The method is reproducible with detailed setup and references.`)
	_, _ = researchEvaluator.Evaluate(ctx, researchIn, researchOut)
	_, _ = researchEvaluator.BatchEvaluate(ctx, []struct {
		Input  *evaluation.EvalInput
		Output *evaluation.EvalOutput
	}{
		{Input: researchIn, Output: researchOut},
		{Input: researchIn, Output: researchOut},
	})

	fmt.Println("  ✓ Evaluation subsystem paths wired")
}

func demoExecutionSubsystem(ctx context.Context, logger *zap.Logger) {
	fmt.Println("\n--- 9. Execution Subsystem ---")

	config := execution.DefaultSandboxConfig()
	config.AllowedLanguages = []execution.Language{
		execution.LangPython,
		execution.Language("unknown"),
	}

	req := &execution.ExecutionRequest{
		ID:       "exec-demo",
		Language: execution.Language("unknown"),
		Code:     "print('demo')",
		Timeout:  100 * time.Millisecond,
	}

	dockerBackend := execution.NewDockerBackend(logger)
	_ = dockerBackend.Name()
	sandbox := execution.NewSandboxExecutor(config, dockerBackend, logger)
	_, _ = sandbox.Execute(ctx, req)
	_ = sandbox.Stats()
	_ = sandbox.Cleanup()
	_ = dockerBackend.Cleanup()

	customDocker := execution.NewDockerBackendWithConfig(logger, execution.DockerBackendConfig{
		ContainerPrefix: "demo_",
		CleanupOnExit:   true,
		CustomImages: map[execution.Language]string{
			execution.LangPython: "python:3.12-slim",
		},
	})
	_ = customDocker.Name()
	_, _ = customDocker.Execute(ctx, req, config)
	_ = customDocker.Cleanup()

	realDocker := execution.NewRealDockerBackend(logger)
	_, _ = realDocker.Execute(ctx, req, config)
	_ = realDocker.Cleanup()

	processBackend := execution.NewProcessBackend(logger)
	_ = processBackend.Name()
	_, _ = processBackend.Execute(ctx, &execution.ExecutionRequest{
		ID:       "proc-disabled",
		Language: execution.LangPython,
		Code:     "print('process')",
	}, config)
	_ = processBackend.Cleanup()

	realProcess := execution.NewRealProcessBackend(logger, false)
	_, _ = realProcess.Execute(ctx, &execution.ExecutionRequest{
		ID:       "real-proc-disabled",
		Language: execution.LangPython,
		Code:     "print('safe')",
	}, config)

	processEnabled := execution.NewRealProcessBackend(logger, true)
	_, _ = processEnabled.Execute(ctx, &execution.ExecutionRequest{
		ID:       "real-proc-enabled",
		Language: execution.LangPython,
		Code:     "print('ok')",
	}, config)

	tool := execution.NewSandboxTool(sandbox, logger)
	payload, _ := json.Marshal(execution.ExecutionRequest{
		ID:       "tool-demo",
		Language: execution.LangPython,
		Code:     "print('tool')",
	})
	_, _ = tool.Execute(ctx, payload)

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	_ = execution.PullImage(cancelCtx, "alpine:latest", logger)
	_ = execution.EnsureImages(cancelCtx, []execution.Language{execution.LangPython}, logger)

	step := execution.NewRetrievalStep(&demoRetriever{}, &demoReranker{}, logger)
	stepCtx := types.WithTraceID(ctx, "trace-exec")
	stepCtx = types.WithRunID(stepCtx, "run-exec")
	stepCtx = types.WithSpanID(stepCtx, "span-exec")
	_, _ = step.Execute(stepCtx, execution.RetrievalStepRequest{
		Query: "retrieve execution docs",
		TopK:  3,
	})

	fmt.Println("  ✓ Execution subsystem paths wired")
}

func findFreePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 18765
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

type demoCapabilityProvider struct {
	card *a2a.AgentCard
}

func (p *demoCapabilityProvider) ID() string   { return p.card.Name }
func (p *demoCapabilityProvider) Name() string { return p.card.Name }
func (p *demoCapabilityProvider) GetCapabilities() []a2a.Capability {
	return p.card.Capabilities
}
func (p *demoCapabilityProvider) GetAgentCard() *a2a.AgentCard { return p.card }

type demoAgentExecutor struct{}

func (e *demoAgentExecutor) ExecuteCapability(ctx context.Context, agentID string, capability string, input any) (any, error) {
	return map[string]any{
		"agent_id":   agentID,
		"capability": capability,
		"input":      input,
	}, nil
}

type demoEvalExecutor struct{}

func (e *demoEvalExecutor) Execute(ctx context.Context, input string) (output string, tokens int, err error) {
	return input, len(input), nil
}

type demoRetriever struct{}

func (r *demoRetriever) Retrieve(ctx context.Context, query string, topK int) ([]types.RetrievalRecord, error) {
	records := []types.RetrievalRecord{
		{DocID: "r1", Content: "execution and sandbox basics", Score: 0.9},
		{DocID: "r2", Content: "docker backend usage", Score: 0.8},
		{DocID: "r3", Content: "process backend usage", Score: 0.7},
	}
	if topK > 0 && topK < len(records) {
		return records[:topK], nil
	}
	return records, nil
}

type demoReranker struct{}

func (r *demoReranker) Rerank(ctx context.Context, query string, records []types.RetrievalRecord) ([]types.RetrievalRecord, error) {
	if len(records) <= 1 {
		return records, nil
	}
	return append([]types.RetrievalRecord{records[len(records)-1]}, records[:len(records)-1]...), nil
}

type demoJudgeProvider struct{}

func (p *demoJudgeProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	content := `{
  "dimensions": {
    "relevance": {"score": 8.5, "reasoning": "relevant"},
    "accuracy": {"score": 9.0, "reasoning": "accurate"},
    "completeness": {"score": 8.0, "reasoning": "complete"},
    "clarity": {"score": 8.8, "reasoning": "clear"}
  },
  "overall_score": 8.6,
  "reasoning": "solid response",
  "confidence": 0.9
}`
	return &llm.ChatResponse{
		Model: req.Model,
		Choices: []llm.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    llm.RoleAssistant,
					Content: content,
				},
			},
		},
		Usage: llm.ChatUsage{TotalTokens: 50},
	}, nil
}

func (p *demoJudgeProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (p *demoJudgeProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (p *demoJudgeProvider) Name() string { return "demo-judge-provider" }

func (p *demoJudgeProvider) SupportsNativeFunctionCalling() bool { return false }

func (p *demoJudgeProvider) ListModels(ctx context.Context) ([]llm.Model, error) { return nil, nil }

func (p *demoJudgeProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

type demoMemoryManager struct {
	mu      sync.RWMutex
	records map[string]memorycore.MemoryRecord
}

func newDemoMemoryManager() *demoMemoryManager {
	return &demoMemoryManager{
		records: make(map[string]memorycore.MemoryRecord),
	}
}

func (m *demoMemoryManager) Save(ctx context.Context, rec memorycore.MemoryRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("mem_%d", time.Now().UnixNano())
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	m.records[rec.ID] = rec
	return nil
}

func (m *demoMemoryManager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.records, id)
	return nil
}

func (m *demoMemoryManager) Clear(ctx context.Context, agentID string, kind memorycore.MemoryKind) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, rec := range m.records {
		if rec.AgentID == agentID && rec.Kind == kind {
			delete(m.records, id)
		}
	}
	return nil
}

func (m *demoMemoryManager) LoadRecent(ctx context.Context, agentID string, kind memorycore.MemoryKind, limit int) ([]memorycore.MemoryRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	results := make([]memorycore.MemoryRecord, 0)
	for _, rec := range m.records {
		if rec.AgentID == agentID && rec.Kind == kind {
			results = append(results, rec)
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (m *demoMemoryManager) Search(ctx context.Context, agentID string, query string, topK int) ([]memorycore.MemoryRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	results := make([]memorycore.MemoryRecord, 0)
	q := strings.ToLower(query)
	for _, rec := range m.records {
		if rec.AgentID != agentID {
			continue
		}
		if q == "" || strings.Contains(strings.ToLower(rec.Content), q) {
			results = append(results, rec)
		}
	}
	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func (m *demoMemoryManager) Get(ctx context.Context, id string) (*memorycore.MemoryRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rec, ok := m.records[id]
	if !ok {
		return nil, fmt.Errorf("memory record not found: %s", id)
	}
	copied := rec
	return &copied, nil
}

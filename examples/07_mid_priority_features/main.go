package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence/conversation"
	"github.com/BaSui01/agentflow/agent/team"
	"github.com/BaSui01/agentflow/agent/adapters/declarative"
	"github.com/BaSui01/agentflow/agent/adapters/handoff"
	"github.com/BaSui01/agentflow/agent/integration/hosted"
	"github.com/BaSui01/agentflow/agent/capabilities/streaming"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== Medium Priority Features Demo ===")

	// 1. Hosted Tools
	demoHostedTools(logger)

	// 2. Agent Handoff
	demoAgentHandoff(logger)

	// 3. Role-based Crews
	demoCrews(logger)

	// 4. Declarative Agent Definition
	demoDeclarative(logger)

	// 5. Conversation Mode
	demoConversation(logger)

	// 6. Bidirectional Streaming
	demoBidirectionalStreaming(logger)

	// 7. Tracing Integration
	demoTracing(logger)

	fmt.Println("\n=== All Medium Priority Features Demonstrated ===")
}

func demoHostedTools(logger *zap.Logger) {
	fmt.Println("1. Hosted Tools (OpenAI SDK-style)")
	fmt.Println("-----------------------------------")

	registry := hosted.NewToolRegistry(logger)
	registry.Use(
		hosted.WithTimeout(5*time.Second),
		hosted.WithLogging(logger),
		hosted.WithMetrics(func(name string, duration time.Duration, err error) {}),
	)

	fallbackCandidates := buildHostedWebSearchCandidates()
	fallbackNote := describeHostedWebSearchFallback(fallbackCandidates)

	// Register file search tool (in-memory mock store).
	fileStore := &mockFileSearchStore{}
	fileTool := hosted.NewFileSearchTool(fileStore, 3)
	registry.Register(fileTool)
	_ = fileTool.Type()
	_ = fileTool.Name()
	_ = fileTool.Description()
	_ = fileTool.Schema()
	_ = fileStore.Index(context.Background(), "f1", []byte("agentflow hosted file search"))

	fileArgs, _ := json.Marshal(map[string]any{
		"query":       "agentflow",
		"max_results": 2,
	})
	_, _ = registry.Execute(context.Background(), fileTool.Name(), fileArgs)

	searchArgs, _ := json.Marshal(map[string]any{
		"query":       "Go programming language",
		"max_results": 3,
		"language":    "en",
	})
	searchResult, searchErr := executeHostedWebSearchWithFallback(context.Background(), logger, fallbackCandidates, searchArgs)

	fmt.Printf("   Registered tool: %s - %s\n", hosted.ToolTypeWebSearch, "Search the web for information. Returns a list of relevant results with titles, URLs, and snippets.")
	fmt.Printf("   Tool type: %s\n", hosted.ToolTypeWebSearch)
	fmt.Printf("   Web search fallback chain: %s\n", fallbackNote)
	if searchErr != nil {
		fmt.Printf("   Web search execution failed: %v\n", searchErr)
	} else {
		fmt.Printf("   Web search provider: %s\n", searchResult.ProviderNote)
		fmt.Printf("   Web search results: %d (%s)\n", searchResult.Response.TotalCount, searchResult.Response.Duration)
		for i, item := range searchResult.Response.Results {
			if i >= 2 {
				break
			}
			fmt.Printf("     - %s | %s\n", item.Title, item.URL)
		}
	}
	for _, attempt := range searchResult.Attempts {
		if attempt.Error != "" {
			fmt.Printf("   Attempt: %s -> error=%s\n", attempt.Provider, attempt.Error)
			continue
		}
		fmt.Printf("   Attempt: %s -> results=%d (%s)\n", attempt.Provider, attempt.Results, attempt.Duration)
	}
	fmt.Printf("   Registered tools (registry): %d\n\n", len(registry.List()))
}

type demoWebSearchResponse struct {
	Query      string                    `json:"query"`
	Results    []demoWebSearchResultItem `json:"results"`
	TotalCount int                       `json:"total_count"`
	Duration   string                    `json:"duration"`
}

type demoWebSearchResultItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type hostedWebSearchCandidate struct {
	Config hosted.ToolProviderConfig
	Note   string
}

type hostedWebSearchAttempt struct {
	Provider string
	Results  int
	Duration string
	Error    string
}

type hostedWebSearchRunResult struct {
	Response     demoWebSearchResponse
	ProviderNote string
	Attempts     []hostedWebSearchAttempt
}

func buildHostedWebSearchCandidates() []hostedWebSearchCandidate {
	candidates := make([]hostedWebSearchCandidate, 0, 4)
	if apiKey := strings.TrimSpace(os.Getenv("TAVILY_API_KEY")); apiKey != "" {
		candidates = append(candidates, hostedWebSearchCandidate{
			Config: hosted.ToolProviderConfig{
				Provider:       string(hosted.ToolProviderTavily),
				APIKey:         apiKey,
				BaseURL:        strings.TrimSpace(os.Getenv("TAVILY_BASE_URL")),
				TimeoutSeconds: 15,
				Enabled:        true,
			},
			Note: "tavily (检测到 TAVILY_API_KEY)",
		})
	}

	if apiKey := strings.TrimSpace(os.Getenv("FIRECRAWL_API_KEY")); apiKey != "" {
		candidates = append(candidates, hostedWebSearchCandidate{
			Config: hosted.ToolProviderConfig{
				Provider:       string(hosted.ToolProviderFirecrawl),
				APIKey:         apiKey,
				BaseURL:        strings.TrimSpace(os.Getenv("FIRECRAWL_BASE_URL")),
				TimeoutSeconds: 15,
				Enabled:        true,
			},
			Note: "firecrawl (检测到 FIRECRAWL_API_KEY)",
		})
	}

	if baseURL := strings.TrimSpace(os.Getenv("SEARXNG_BASE_URL")); baseURL != "" {
		candidates = append(candidates, hostedWebSearchCandidate{
			Config: hosted.ToolProviderConfig{
				Provider:       string(hosted.ToolProviderSearXNG),
				BaseURL:        baseURL,
				TimeoutSeconds: 15,
				Enabled:        true,
			},
			Note: "searxng (检测到 SEARXNG_BASE_URL)",
		})
	}

	candidates = append(candidates, hostedWebSearchCandidate{
		Config: hosted.ToolProviderConfig{
			Provider:       string(hosted.ToolProviderDuckDuckGo),
			TimeoutSeconds: 15,
			Enabled:        true,
		},
		Note: "duckduckgo (默认免 Key 回退)",
	})

	return candidates
}

func describeHostedWebSearchFallback(candidates []hostedWebSearchCandidate) string {
	notes := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		notes = append(notes, candidate.Note)
	}
	return strings.Join(notes, " -> ")
}

func executeHostedWebSearchWithFallback(ctx context.Context, logger *zap.Logger, candidates []hostedWebSearchCandidate, searchArgs json.RawMessage) (*hostedWebSearchRunResult, error) {
	result := &hostedWebSearchRunResult{
		Attempts: make([]hostedWebSearchAttempt, 0, len(candidates)),
	}

	var lastErr error
	for _, candidate := range candidates {
		webSearch, err := hosted.NewProviderBackedWebSearchHostedTool(candidate.Config, logger)
		if err != nil {
			lastErr = err
			result.Attempts = append(result.Attempts, hostedWebSearchAttempt{
				Provider: candidate.Note,
				Error:    err.Error(),
			})
			continue
		}

		searchRaw, err := webSearch.Execute(ctx, searchArgs)
		if err != nil {
			lastErr = err
			result.Attempts = append(result.Attempts, hostedWebSearchAttempt{
				Provider: candidate.Note,
				Error:    err.Error(),
			})
			continue
		}

		var resp demoWebSearchResponse
		if err := json.Unmarshal(searchRaw, &resp); err != nil {
			lastErr = err
			result.Attempts = append(result.Attempts, hostedWebSearchAttempt{
				Provider: candidate.Note,
				Error:    err.Error(),
			})
			continue
		}

		result.Attempts = append(result.Attempts, hostedWebSearchAttempt{
			Provider: candidate.Note,
			Results:  resp.TotalCount,
			Duration: resp.Duration,
		})
		if resp.TotalCount > 0 {
			result.Response = resp
			result.ProviderNote = candidate.Note
			return result, nil
		}

		lastErr = fmt.Errorf("provider %s returned 0 results", candidate.Config.Provider)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no web search providers available")
	}
	return result, lastErr
}

func demoAgentHandoff(logger *zap.Logger) {
	fmt.Println("2. Agent Handoff Protocol")
	fmt.Println("-------------------------")

	ctx := context.Background()
	manager := handoff.NewHandoffManager(logger)

	// Create mock agent
	agent := &MockHandoffAgent{
		id: "specialist-agent",
		capabilities: []handoff.AgentCapability{
			{Name: "code_review", TaskTypes: []string{"review"}, Priority: 10},
		},
	}
	manager.RegisterAgent(agent)
	manager.RegisterAgent(&MockHandoffAgent{
		id: "review-agent",
		capabilities: []handoff.AgentCapability{
			{Name: "review", TaskTypes: []string{"review"}, Priority: 20},
		},
	})

	fmt.Printf("   Registered agent: %s\n", agent.ID())
	fmt.Printf("   Capabilities: %v\n", agent.Capabilities()[0].TaskTypes)

	task := handoff.Task{
		Type:        "review",
		Description: "Review deployment checklist",
		Input:       map[string]any{"doc": "checklist-v1"},
		Priority:    1,
	}
	found, findErr := manager.FindAgent(task)
	if findErr != nil {
		fmt.Printf("   FindAgent error: %v\n\n", findErr)
		return
	}
	fmt.Printf("   Best agent for task: %s\n", found.ID())

	ho, err := manager.Handoff(ctx, handoff.HandoffOptions{
		FromAgentID: "coordinator-agent",
		Task:        task,
		Context: handoff.HandoffContext{
			ConversationID: "conv-demo",
			Variables:      map[string]any{"priority": "high"},
		},
		Timeout: time.Second,
		Wait:    true,
	})
	if err != nil {
		fmt.Printf("   Handoff error: %v\n\n", err)
		return
	}
	fmt.Printf("   Handoff status: %s (id=%s)\n", ho.Status, ho.ID)

	stored, getErr := manager.GetHandoff(ho.ID)
	if getErr == nil && stored != nil && stored.Result != nil {
		fmt.Printf("   Handoff result: %v\n", stored.Result.Output)
	}

	manager.UnregisterAgent(agent.ID())
	fmt.Printf("   Unregistered agent: %s\n\n", agent.ID())
}

// MockHandoffAgent implements handoff.HandoffAgent for demo.
type MockHandoffAgent struct {
	id           string
	capabilities []handoff.AgentCapability
}

func (a *MockHandoffAgent) ID() string                              { return a.id }
func (a *MockHandoffAgent) Capabilities() []handoff.AgentCapability { return a.capabilities }
func (a *MockHandoffAgent) CanHandle(task handoff.Task) bool        { return true }
func (a *MockHandoffAgent) AcceptHandoff(ctx context.Context, h *handoff.Handoff) error {
	return nil
}
func (a *MockHandoffAgent) ExecuteHandoff(ctx context.Context, h *handoff.Handoff) (*handoff.HandoffResult, error) {
	return &handoff.HandoffResult{Output: "completed"}, nil
}

func demoCrews(logger *zap.Logger) {
	fmt.Println("3. Role-based Crews (CrewAI-style)")
	fmt.Println("----------------------------------")

	sequentialCrew := team.NewCrew(team.CrewConfig{
		Name:        "Research Team",
		Description: "A team for research tasks",
		Process:     team.ProcessSequential,
	}, logger)

	// Add members with roles
	researcher := &MockCrewAgent{id: "researcher", voteFor: "researcher"}
	sequentialCrew.AddMember(researcher, team.Role{
		Name:        "Researcher",
		Description: "Conducts research",
		Goal:        "Find relevant information",
		Skills:      []string{"research", "analysis"},
	})

	writer := &MockCrewAgent{id: "writer", voteFor: "writer"}
	sequentialCrew.AddMember(writer, team.Role{
		Name:        "Writer",
		Description: "Writes content",
		Goal:        "Create clear documentation",
		Skills:      []string{"writing", "editing"},
	})

	// Add task
	sequentialCrew.AddTask(team.CrewTask{
		Description: "Research AI frameworks",
		Expected:    "Summary report",
		Priority:    1,
	})

	seqResult, seqErr := sequentialCrew.Execute(context.Background())
	if seqErr != nil {
		fmt.Printf("   Sequential execute error: %v\n", seqErr)
	}

	hierarchicalCrew := team.NewCrew(team.CrewConfig{
		Name:        "Manager Team",
		Description: "A team for delegated execution",
		Process:     team.ProcessHierarchical,
	}, logger)
	manager := &MockCrewAgent{id: "manager", voteFor: "manager"}
	hierarchicalCrew.AddMember(manager, team.Role{
		Name:            "Manager",
		Description:     "Delegates tasks",
		Goal:            "Complete tasks via delegation",
		AllowDelegation: true,
	})
	hierarchicalCrew.AddMember(&MockCrewAgent{id: "specialist", voteFor: "specialist"}, team.Role{
		Name:        "Specialist",
		Description: "Handles specialist tasks",
		Goal:        "Solve technical tasks",
	})
	hierarchicalCrew.AddTask(team.CrewTask{
		Description: "Build deployment checklist",
		Expected:    "Checklist document",
		Priority:    1,
	})
	hierResult, hierErr := hierarchicalCrew.Execute(context.Background())
	if hierErr != nil {
		fmt.Printf("   Hierarchical execute error: %v\n", hierErr)
	}

	consensusCrew := team.NewCrew(team.CrewConfig{
		Name:        "Consensus Team",
		Description: "A team for consensus voting",
		Process:     team.ProcessConsensus,
	}, logger)
	consensusCrew.AddMember(&MockCrewAgent{id: "judge-a", voteFor: "judge-b"}, team.Role{Name: "Judge A", Goal: "vote"})
	consensusCrew.AddMember(&MockCrewAgent{id: "judge-b", voteFor: "judge-b"}, team.Role{Name: "Judge B", Goal: "vote"})
	consensusCrew.AddTask(team.CrewTask{
		Description: "Choose owner for final report",
		Expected:    "Owner decision",
		Priority:    1,
	})
	conResult, conErr := consensusCrew.Execute(context.Background())
	if conErr != nil {
		fmt.Printf("   Consensus execute error: %v\n", conErr)
	}

	fmt.Printf("   Sequential results: %d\n", len(seqResult.TaskResults))
	fmt.Printf("   Hierarchical results: %d\n", len(hierResult.TaskResults))
	fmt.Printf("   Consensus results: %d\n\n", len(conResult.TaskResults))
}

type mockFileSearchStore struct{}

func (s *mockFileSearchStore) Search(ctx context.Context, query string, limit int) ([]hosted.FileSearchResult, error) {
	results := []hosted.FileSearchResult{
		{FileID: "f1", FileName: "demo.txt", Content: "agentflow hosted tools", Score: 0.91},
		{FileID: "f2", FileName: "notes.txt", Content: "file search example", Score: 0.82},
	}
	if limit > 0 && limit < len(results) {
		return results[:limit], nil
	}
	return results, nil
}

func (s *mockFileSearchStore) Index(ctx context.Context, fileID string, content []byte) error {
	_ = fileID
	_ = content
	return nil
}

// MockCrewAgent implements team.CrewAgent for demo.
type MockCrewAgent struct {
	id      string
	voteFor string
}

func (a *MockCrewAgent) ID() string { return a.id }
func (a *MockCrewAgent) Execute(ctx context.Context, task team.CrewTask) (*team.TaskResult, error) {
	return &team.TaskResult{TaskID: task.ID, Output: "done"}, nil
}
func (a *MockCrewAgent) Negotiate(ctx context.Context, p team.Proposal) (*team.NegotiationResult, error) {
	resp := a.voteFor
	if resp == "" {
		resp = a.id
	}
	return &team.NegotiationResult{Accepted: true, Response: resp}, nil
}

func demoDeclarative(logger *zap.Logger) {
	fmt.Println("4. Declarative Agent Definition")
	fmt.Println("-------------------------------")

	loader := declarative.NewYAMLLoader()
	yamlDef := []byte(`
id: declarative-demo
name: DeclarativeDemo
model: gpt-4o-mini
provider: openai
temperature: 0.2
max_tokens: 512
system_prompt: You are a concise assistant.
tools: [search]
features:
  enable_reflection: true
  max_react_iterations: 2
metadata:
  env: demo
`)

	defFromYAML, err := loader.LoadBytes(yamlDef, "yaml")
	if err != nil {
		fmt.Printf("   Load YAML failed: %v\n\n", err)
		return
	}

	jsonDef := []byte(`{"id":"json-demo","name":"JSONDemo","model":"gpt-4o-mini","temperature":0.1}`)
	_, _ = loader.LoadBytes(jsonDef, "json")

	tmpFile, err := os.CreateTemp("", "agent-def-*.yaml")
	if err == nil {
		_, _ = tmpFile.Write(yamlDef)
		_ = tmpFile.Close()
		_, _ = loader.LoadFile(tmpFile.Name())
		_ = os.Remove(tmpFile.Name())
	}

	factory := declarative.NewAgentFactory(logger)
	if err := factory.Validate(defFromYAML); err != nil {
		fmt.Printf("   Validate failed: %v\n\n", err)
		return
	}
	cfg := factory.ToAgentConfig(defFromYAML)

	fmt.Printf("   Loaded definition: %s\n", defFromYAML.Name)
	fmt.Printf("   Runtime model: %s\n", cfg.LLM.Model)
	fmt.Printf("   Runtime tools: %d\n\n", len(cfg.Runtime.Tools))
}

func demoConversation(logger *zap.Logger) {
	fmt.Println("4. Conversation Mode (AutoGen-style)")
	fmt.Println("------------------------------------")

	agents := []conversation.ConversationAgent{
		&MockConvAgent{id: "assistant", name: "Assistant"},
		&MockConvAgent{id: "critic", name: "Critic"},
	}

	conv := conversation.NewConversation(
		conversation.ModeRoundRobin,
		agents,
		conversation.DefaultConversationConfig(),
		logger,
	)

	result, err := conv.Start(context.Background(), "请开始协作讨论")
	if err != nil {
		fmt.Printf("   Start conversation failed: %v\n\n", err)
		return
	}

	messages := conv.GetMessages()

	chatManager := conversation.NewGroupChatManager(logger)
	managed := chatManager.CreateChat(agents, conversation.DefaultConversationConfig())
	_, _ = chatManager.GetChat(managed.ID)

	tree := conversation.NewConversationTree("conv-tree-demo")
	tree.AddMessage(types.Message{Role: "user", Content: "需求分析"})
	_, _ = tree.Fork("alt")
	_ = tree.SwitchBranch("alt")
	tree.AddMessage(types.Message{Role: "assistant", Content: "备选方案"})
	_ = tree.SwitchBranch("main")
	_ = tree.MergeBranch("alt")
	tree.Snapshot("v1")
	_ = tree.RollbackN(1)
	_ = tree.RestoreSnapshot("v1")
	_ = tree.DeleteBranch("alt")
	_ = tree.FindSnapshot("v1")
	_ = tree.GetCurrentState()
	_ = tree.GetHistory()
	_ = tree.ListBranches()
	_, _ = tree.Export()
	if data, exportErr := tree.Export(); exportErr == nil {
		_, _ = conversation.Import(data)
	}
	_ = tree.GetMessages()

	fmt.Printf("   Conversation ID: %s\n", conv.ID)
	fmt.Printf("   Mode: %s\n", conv.Mode)
	fmt.Printf("   Agents: %d\n", len(conv.Agents))
	fmt.Printf("   Max rounds: %d\n\n", conv.Config.MaxRounds)
	fmt.Printf("   Termination: %s (rounds=%d)\n", result.TerminationReason, result.TotalRounds)
	fmt.Printf("   Message count: %d\n\n", len(messages))
}

// MockConvAgent implements conversation.ConversationAgent for demo.
type MockConvAgent struct {
	id   string
	name string
}

func (a *MockConvAgent) ID() string           { return a.id }
func (a *MockConvAgent) Name() string         { return a.name }
func (a *MockConvAgent) SystemPrompt() string { return "You are " + a.name }
func (a *MockConvAgent) Reply(ctx context.Context, msgs []conversation.ChatMessage) (*conversation.ChatMessage, error) {
	return &conversation.ChatMessage{
		Role:    "assistant",
		Content: fmt.Sprintf("[%s] Response to: %s", a.name, msgs[len(msgs)-1].Content),
	}, nil
}
func (a *MockConvAgent) ShouldTerminate(msgs []conversation.ChatMessage) bool {
	return len(msgs) > 5
}

func demoBidirectionalStreaming(logger *zap.Logger) {
	fmt.Println("5. Bidirectional Streaming")
	fmt.Println("--------------------------")

	manager := streaming.NewStreamManager(logger)

	config := streaming.DefaultStreamConfig()
	stream := manager.CreateStream(config, nil, nil, nil)

	fmt.Printf("   Stream ID: %s\n", stream.ID)
	fmt.Printf("   Buffer size: %d\n", config.BufferSize)
	fmt.Printf("   Max latency: %dms\n", config.MaxLatencyMS)
	fmt.Printf("   VAD enabled: %v\n\n", config.EnableVAD)
}

func demoTracing(logger *zap.Logger) {
	fmt.Println("6. Tracing Integration (LangSmith-style)")
	fmt.Println("----------------------------------------")

	tracer := observability.NewTracer(observability.TracerConfig{
		ServiceName: "demo-agent",
	}, nil, logger)

	ctx := context.Background()
	ctx, run := tracer.StartRun(ctx, "demo-conversation")

	// Trace an LLM call
	ctx, trace := tracer.StartTrace(ctx, observability.TraceTypeLLM, "gpt-4", "Hello")
	tracer.EndTrace(ctx, trace.ID, "Hi there!", nil)

	// Trace a tool call
	ctx, toolTrace := tracer.StartTrace(ctx, observability.TraceTypeTool, "search", map[string]string{"q": "test"})
	tracer.EndTrace(ctx, toolTrace.ID, []string{"result1", "result2"}, nil)

	tracer.EndRun(ctx, run.ID, "completed")

	fmt.Printf("   Run ID: %s\n", run.ID)
	fmt.Printf("   Traces: %d\n", len(run.Traces))
	fmt.Printf("   Status: %s\n\n", run.Status)

	// Demo conversation tracer
	convTracer := observability.NewConversationTracer(tracer)
	_, conv := convTracer.StartConversation(ctx, "test-chat")
	fmt.Printf("   Conversation trace ID: %s\n", conv.ID)
}

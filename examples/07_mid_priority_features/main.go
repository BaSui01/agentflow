// Package main demonstrates medium priority features.
package main

import (
	"context"
	"fmt"

	"github.com/BaSui01/agentflow/agent/conversation"
	"github.com/BaSui01/agentflow/agent/crews"
	"github.com/BaSui01/agentflow/agent/handoff"
	"github.com/BaSui01/agentflow/agent/hosted"
	"github.com/BaSui01/agentflow/agent/streaming"
	"github.com/BaSui01/agentflow/llm/observability"
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

	// 4. Conversation Mode
	demoConversation(logger)

	// 5. Bidirectional Streaming
	demoBidirectionalStreaming(logger)

	// 6. Tracing Integration
	demoTracing(logger)

	fmt.Println("\n=== All Medium Priority Features Demonstrated ===")
}

func demoHostedTools(logger *zap.Logger) {
	fmt.Println("1. Hosted Tools (OpenAI SDK-style)")
	fmt.Println("-----------------------------------")

	registry := hosted.NewToolRegistry(logger)

	// Register web search tool
	webSearch := hosted.NewWebSearchTool(hosted.WebSearchConfig{
		APIKey:     "demo-key",
		Endpoint:   "https://api.search.example.com/search",
		MaxResults: 5,
	})
	registry.Register(webSearch)

	fmt.Printf("   Registered tool: %s - %s\n", webSearch.Name(), webSearch.Description())
	fmt.Printf("   Tool type: %s\n", webSearch.Type())
	fmt.Printf("   Total tools: %d\n\n", len(registry.List()))
}

func demoAgentHandoff(logger *zap.Logger) {
	fmt.Println("2. Agent Handoff Protocol")
	fmt.Println("-------------------------")

	manager := handoff.NewHandoffManager(logger)

	// Create mock agent
	agent := &MockHandoffAgent{
		id: "specialist-agent",
		capabilities: []handoff.AgentCapability{
			{Name: "code_review", TaskTypes: []string{"review"}, Priority: 10},
		},
	}
	manager.RegisterAgent(agent)

	fmt.Printf("   Registered agent: %s\n", agent.ID())
	fmt.Printf("   Capabilities: %v\n\n", agent.Capabilities()[0].TaskTypes)
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

	crew := crews.NewCrew(crews.CrewConfig{
		Name:        "Research Team",
		Description: "A team for research tasks",
		Process:     crews.ProcessSequential,
	}, logger)

	// Add members with roles
	researcher := &MockCrewAgent{id: "researcher"}
	crew.AddMember(researcher, crews.Role{
		Name:        "Researcher",
		Description: "Conducts research",
		Goal:        "Find relevant information",
		Skills:      []string{"research", "analysis"},
	})

	writer := &MockCrewAgent{id: "writer"}
	crew.AddMember(writer, crews.Role{
		Name:        "Writer",
		Description: "Writes content",
		Goal:        "Create clear documentation",
		Skills:      []string{"writing", "editing"},
	})

	// Add task
	crew.AddTask(crews.CrewTask{
		Description: "Research AI frameworks",
		Expected:    "Summary report",
		Priority:    1,
	})

	fmt.Printf("   Crew: %s\n", crew.Name)
	fmt.Printf("   Process: %s\n", crew.Process)
	fmt.Printf("   Members: %d\n", len(crew.Members))
	fmt.Printf("   Tasks: %d\n\n", len(crew.Tasks))
}

// MockCrewAgent implements crews.CrewAgent for demo.
type MockCrewAgent struct {
	id string
}

func (a *MockCrewAgent) ID() string { return a.id }
func (a *MockCrewAgent) Execute(ctx context.Context, task crews.CrewTask) (*crews.TaskResult, error) {
	return &crews.TaskResult{TaskID: task.ID, Output: "done"}, nil
}
func (a *MockCrewAgent) Negotiate(ctx context.Context, p crews.Proposal) (*crews.NegotiationResult, error) {
	return &crews.NegotiationResult{Accepted: true}, nil
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

	fmt.Printf("   Conversation ID: %s\n", conv.ID)
	fmt.Printf("   Mode: %s\n", conv.Mode)
	fmt.Printf("   Agents: %d\n", len(conv.Agents))
	fmt.Printf("   Max rounds: %d\n\n", conv.Config.MaxRounds)
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

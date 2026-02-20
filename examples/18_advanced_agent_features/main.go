package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/agent/deliberation"
	"github.com/BaSui01/agentflow/agent/federation"
	"github.com/BaSui01/agentflow/agent/longrunning"
	"github.com/BaSui01/agentflow/agent/skills"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	fmt.Println("=== Advanced Agent Features Demo ===")

	// 1. Federated Orchestration
	demoFederation(logger)

	// 2. Deliberation Mode
	demoDeliberation(logger)

	// 3. Long-Running Executor
	demoLongRunning(logger)

	// 4. Skills Registry
	demoSkillsRegistry(logger)

	fmt.Println("\n=== All Advanced Features Demonstrated ===")
}

func demoFederation(logger *zap.Logger) {
	fmt.Println("1. Federated Orchestration")
	fmt.Println("--------------------------")

	orch := federation.NewOrchestrator(federation.FederationConfig{
		NodeID:   "node-1",
		NodeName: "Primary Node",
	}, logger)

	// Register nodes
	orch.RegisterNode(&federation.FederatedNode{
		ID:           "node-2",
		Name:         "Worker Node",
		Endpoint:     "https://worker.example.com",
		Capabilities: []string{"compute", "storage"},
	})

	orch.RegisterNode(&federation.FederatedNode{
		ID:           "node-3",
		Name:         "Analytics Node",
		Endpoint:     "https://analytics.example.com",
		Capabilities: []string{"analytics", "ml"},
	})

	nodes := orch.ListNodes()
	fmt.Printf("   Registered nodes: %d\n", len(nodes))
	for _, n := range nodes {
		fmt.Printf("   - %s: %v\n", n.Name, n.Capabilities)
	}
	fmt.Println()
}

func demoDeliberation(logger *zap.Logger) {
	fmt.Println("2. Agent Deliberation Mode")
	fmt.Println("--------------------------")

	config := deliberation.DefaultDeliberationConfig()
	engine := deliberation.NewEngine(config, &MockReasoner{}, logger)

	fmt.Printf("   Mode: %s\n", engine.GetMode())
	fmt.Printf("   Max thinking time: %v\n", config.MaxThinkingTime)
	fmt.Printf("   Min confidence: %.2f\n", config.MinConfidence)
	fmt.Printf("   Self-critique enabled: %v\n", config.EnableSelfCritique)

	// Switch modes
	engine.SetMode(deliberation.ModeImmediate)
	fmt.Printf("   Switched to: %s\n", engine.GetMode())

	engine.SetMode(deliberation.ModeDeliberate)
	fmt.Printf("   Switched to: %s\n\n", engine.GetMode())
}

// MockReasoner implements deliberation.Reasoner for demo.
type MockReasoner struct{}

func (r *MockReasoner) Think(ctx context.Context, prompt string) (string, float64, error) {
	return "Analyzed the task and determined best approach", 0.85, nil
}

func demoLongRunning(logger *zap.Logger) {
	fmt.Println("3. Long-Running Agent Executor")
	fmt.Println("------------------------------")

	config := longrunning.DefaultExecutorConfig()
	executor := longrunning.NewExecutor(config, logger)

	// Create steps
	steps := []longrunning.StepFunc{
		func(ctx context.Context, state any) (any, error) {
			return map[string]any{"step": 1, "data": "initialized"}, nil
		},
		func(ctx context.Context, state any) (any, error) {
			return map[string]any{"step": 2, "data": "processed"}, nil
		},
		func(ctx context.Context, state any) (any, error) {
			return map[string]any{"step": 3, "data": "completed"}, nil
		},
	}

	exec := executor.CreateExecution("data-pipeline", steps)
	fmt.Printf("   Execution ID: %s\n", exec.ID)
	fmt.Printf("   Total steps: %d\n", exec.TotalSteps)
	fmt.Printf("   Checkpoint interval: %v\n", config.CheckpointInterval)
	fmt.Printf("   Auto-resume: %v\n\n", config.AutoResume)
}

func demoSkillsRegistry(logger *zap.Logger) {
	fmt.Println("4. Agent Skills Registry")
	fmt.Println("------------------------")

	registry := skills.NewRegistry(logger)

	// Register skills
	registry.Register(&skills.SkillDefinition{
		Name:        "web_search",
		Category:    skills.CategoryResearch,
		Description: "Search the web for information",
		Tags:        []string{"search", "web", "research"},
	}, func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"result": "search results"})
	})

	registry.Register(&skills.SkillDefinition{
		Name:        "code_review",
		Category:    skills.CategoryCoding,
		Description: "Review code for issues and improvements",
		Tags:        []string{"code", "review", "quality"},
	}, func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"result": "code review complete"})
	})

	registry.Register(&skills.SkillDefinition{
		Name:        "data_analysis",
		Category:    skills.CategoryData,
		Description: "Analyze data and generate insights",
		Tags:        []string{"data", "analysis", "insights"},
	}, func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]string{"result": "analysis complete"})
	})

	allSkills := registry.ListAll()
	fmt.Printf("   Total skills: %d\n", len(allSkills))

	codingSkills := registry.ListByCategory(skills.CategoryCoding)
	fmt.Printf("   Coding skills: %d\n", len(codingSkills))

	searchResults := registry.Search("", []string{"research"})
	fmt.Printf("   Research-tagged skills: %d\n", len(searchResults))

	// Invoke a skill
	if skill, ok := registry.GetByName("web_search"); ok {
		fmt.Printf("   Found skill: %s (%s)\n", skill.Definition.Name, skill.Definition.Category)
	}
}

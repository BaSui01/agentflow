// 示例 17：高优先级功能演示
// 演示内容：产物管理、HITL 中断、OpenAPI 工具、部署、增强检查点与可视化构建
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent/artifacts"
	"github.com/BaSui01/agentflow/agent/deployment"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/tools/openapi"
	"github.com/BaSui01/agentflow/workflow"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	fmt.Println("=== AgentFlow High Priority Features Demo ===")

	// 1. Artifacts Management
	demoArtifacts(ctx, logger)

	// 2. Human-in-the-Loop Interrupts
	demoHITL(ctx, logger)

	// 3. OpenAPI Tool Generation
	demoOpenAPITools(ctx, logger)

	// 4. Cloud Deployment
	demoDeployment(ctx, logger)

	// 5. Enhanced Checkpoints
	demoCheckpoints(ctx, logger)

	// 6. Visual Workflow Builder
	demoVisualBuilder(ctx, logger)

	fmt.Println("\n=== All demos completed ===")
}

func demoArtifacts(ctx context.Context, logger *zap.Logger) {
	fmt.Println("1. Artifacts Management")

	store, _ := artifacts.NewFileStore("./test_artifacts")
	manager := artifacts.NewManager(artifacts.DefaultManagerConfig(), store, logger)

	// Create artifact
	data := bytes.NewReader([]byte("Hello, this is test artifact content"))
	artifact, err := manager.Create(ctx, "test.txt", artifacts.ArtifactTypeFile, data,
		artifacts.WithTags("demo", "test"),
		artifacts.WithCreatedBy("demo_user"),
	)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	fmt.Printf("   Created artifact: %s (size: %d bytes)\n", artifact.ID, artifact.Size)

	// List artifacts
	list, _ := manager.List(ctx, artifacts.ArtifactQuery{Tags: []string{"demo"}})
	fmt.Printf("   Found %d artifacts with 'demo' tag\n\n", len(list))
}

func demoHITL(ctx context.Context, logger *zap.Logger) {
	fmt.Println("2. Human-in-the-Loop Interrupts")

	store := hitl.NewInMemoryInterruptStore()
	manager := hitl.NewInterruptManager(store, logger)

	// Simulate async approval (auto-approve after 100ms)
	go func() {
		time.Sleep(100 * time.Millisecond)
		interrupts := manager.GetPendingInterrupts("")
		for _, i := range interrupts {
			manager.ResolveInterrupt(ctx, i.ID, &hitl.Response{
				Approved: true,
				Comment:  "Auto-approved for demo",
			})
		}
	}()

	// Create interrupt
	response, err := manager.CreateInterrupt(ctx, hitl.InterruptOptions{
		WorkflowID:  "wf_demo",
		NodeID:      "node_1",
		Type:        hitl.InterruptTypeApproval,
		Title:       "Approve Action",
		Description: "Please approve this demo action",
		Timeout:     5 * time.Second,
	})

	if err != nil {
		fmt.Printf("   Error: %v\n", err)
	} else {
		fmt.Printf("   Interrupt resolved: approved=%v, comment=%s\n\n", response.Approved, response.Comment)
	}
}

func demoOpenAPITools(ctx context.Context, logger *zap.Logger) {
	fmt.Println("3. OpenAPI Tool Generation")

	generator := openapi.NewGenerator(openapi.GeneratorConfig{}, logger)

	// Create sample spec
	spec := &openapi.OpenAPISpec{
		OpenAPI: "3.0.0",
		Info:    openapi.Info{Title: "Demo API", Version: "1.0"},
		Servers: []openapi.Server{{URL: "https://api.example.com"}},
		Paths: map[string]openapi.PathItem{
			"/users": {
				Get: &openapi.Operation{
					OperationID: "listUsers",
					Summary:     "List all users",
					Parameters: []openapi.Parameter{
						{Name: "limit", In: "query", Schema: &openapi.JSONSchema{Type: "integer"}},
					},
				},
			},
		},
	}

	tools, _ := generator.GenerateTools(spec, openapi.GenerateOptions{})
	fmt.Printf("   Generated %d tools from OpenAPI spec\n", len(tools))
	for _, t := range tools {
		fmt.Printf("   - %s: %s\n", t.Name, t.Description)
	}
	fmt.Println()
}

func demoDeployment(ctx context.Context, logger *zap.Logger) {
	fmt.Println("4. Cloud Deployment")

	deployer := deployment.NewDeployer(logger)

	// Export K8s manifest (without actual deployment)
	dep := &deployment.Deployment{
		ID:       "dep_demo",
		Name:     "my-agent",
		AgentID:  "agent_001",
		Replicas: 3,
		Config: deployment.DeploymentConfig{
			Image: "myregistry/agent:v1.0",
			Port:  8080,
		},
		Resources: deployment.ResourceConfig{
			CPURequest:    "100m",
			CPULimit:      "500m",
			MemoryRequest: "128Mi",
			MemoryLimit:   "512Mi",
		},
	}

	deployer.RegisterProvider(deployment.TargetLocal, nil) // Placeholder

	manifest, _ := json.MarshalIndent(map[string]any{
		"name":     dep.Name,
		"replicas": dep.Replicas,
		"image":    dep.Config.Image,
	}, "   ", "  ")
	fmt.Printf("   K8s Deployment Preview:\n   %s\n\n", string(manifest))
}

func demoCheckpoints(ctx context.Context, logger *zap.Logger) {
	fmt.Println("5. Enhanced Checkpoints with Time-Travel")

	store := workflow.NewInMemoryCheckpointStore()
	manager := workflow.NewEnhancedCheckpointManager(store, logger)

	// Create checkpoints
	for i := 1; i <= 3; i++ {
		cp := &workflow.EnhancedCheckpoint{
			ID:       fmt.Sprintf("cp_%d", i),
			ThreadID: "thread_demo",
			Version:  i,
			NodeResults: map[string]any{
				"node_1": fmt.Sprintf("result_v%d", i),
			},
			CreatedAt: time.Now(),
		}
		store.Save(ctx, cp)
	}

	// List versions
	versions, _ := manager.GetHistory(ctx, "thread_demo")
	fmt.Printf("   Created %d checkpoint versions\n", len(versions))

	// Compare versions
	diff, _ := manager.Compare(ctx, "thread_demo", 1, 3)
	fmt.Printf("   Diff v1->v3: changed=%v\n\n", diff.ChangedNodes)
}

func demoVisualBuilder(ctx context.Context, logger *zap.Logger) {
	fmt.Println("6. Visual Workflow Builder")

	// Create visual workflow
	vw := &workflow.VisualWorkflow{
		ID:          "vw_demo",
		Name:        "Demo Workflow",
		Description: "A workflow created with visual builder",
		Nodes: []workflow.VisualNode{
			{ID: "start", Type: workflow.VNodeStart, Label: "Start", Position: workflow.Position{X: 100, Y: 100}},
			{ID: "llm", Type: workflow.VNodeLLM, Label: "LLM Call", Position: workflow.Position{X: 300, Y: 100},
				Config: workflow.NodeConfig{Model: "gpt-4", Prompt: "Hello"}},
			{ID: "end", Type: workflow.VNodeEnd, Label: "End", Position: workflow.Position{X: 500, Y: 100}},
		},
		Edges: []workflow.VisualEdge{
			{ID: "e1", Source: "start", Target: "llm"},
			{ID: "e2", Source: "llm", Target: "end"},
		},
	}

	// Validate
	if err := vw.Validate(); err != nil {
		fmt.Printf("   Validation error: %v\n", err)
		return
	}

	// Build executable DAG
	builder := workflow.NewVisualBuilder()
	dag, err := builder.Build(vw)
	if err != nil {
		fmt.Printf("   Build error: %v\n", err)
		return
	}

	fmt.Printf("   Built DAG workflow: %s\n", dag.Name())
	fmt.Printf("   Nodes: %d, Entry: %s\n", len(dag.Graph().Nodes()), dag.Graph().GetEntry())
}

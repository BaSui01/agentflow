// 示例 17：高优先级功能演示
// 演示内容：产物管理、HITL 中断、OpenAPI 工具、部署、增强检查点与可视化构建
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/BaSui01/agentflow/agent/artifacts"
	"github.com/BaSui01/agentflow/agent/deployment"
	"github.com/BaSui01/agentflow/agent/hitl"
	"github.com/BaSui01/agentflow/agent/k8s"
	"github.com/BaSui01/agentflow/pkg/openapi"
	"github.com/BaSui01/agentflow/workflow"
	"github.com/BaSui01/agentflow/workflow/dsl"
	workflowobs "github.com/BaSui01/agentflow/workflow/observability"
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

	// 4.5 Kubernetes Operator
	demoK8sOperator(ctx, logger)

	// 5. Enhanced Checkpoints
	demoCheckpoints(ctx, logger)

	// 6. Visual Workflow Builder
	demoVisualBuilder(ctx, logger)

	// 7. DSL Parser + Workflow Observability Context
	demoDSLParserAndObservability(ctx)

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
		artifacts.WithMetadata(map[string]any{"scenario": "high-priority-demo"}),
		artifacts.WithMimeType("text/plain"),
		artifacts.WithSessionID("demo-session-17"),
		artifacts.WithTTL(2*time.Minute),
	)
	if err != nil {
		fmt.Printf("   Error: %v\n", err)
		return
	}
	fmt.Printf("   Created artifact: %s (size: %d bytes)\n", artifact.ID, artifact.Size)

	metadata, _ := manager.GetMetadata(ctx, artifact.ID)
	loadedMeta := "missing"
	if metadata != nil {
		loadedMeta = metadata.Name
	}
	loadedArtifact, reader, err := manager.Get(ctx, artifact.ID)
	if err == nil && reader != nil {
		_, _ = io.ReadAll(reader)
		_ = reader.Close()
	}
	fmt.Printf("   Loaded artifact: %s\n", loadedArtifact.ID)
	fmt.Printf("   Metadata name: %s\n", loadedMeta)

	_ = manager.Archive(ctx, artifact.ID)

	v2, err := manager.CreateVersion(ctx, artifact.ID, bytes.NewReader([]byte("v2 content")))
	if err == nil {
		fmt.Printf("   Created artifact version: %s (parent=%s)\n", v2.ID, artifact.ID)
		_ = manager.Delete(ctx, v2.ID)
	}

	// List artifacts
	list, _ := manager.List(ctx, artifacts.ArtifactQuery{Tags: []string{"demo"}})
	fmt.Printf("   Found %d artifacts with 'demo' tag\n", len(list))

	deleted, cleanupErr := manager.Cleanup(ctx)
	fmt.Printf("   Cleanup deleted: %d (err=%v)\n", deleted, cleanupErr)
	_ = manager.Delete(ctx, artifact.ID)
	fmt.Println()
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
	callbackEvents := 0
	deployer.SetCallbacks(deployment.DeploymentEventCallbacks{
		OnDeploy: func(d *deployment.Deployment) {
			callbackEvents++
			_ = d.ID
		},
		OnDelete: func(deploymentID string) {
			callbackEvents++
			_ = deploymentID
		},
		OnScale: func(deploymentID string, from, to int) {
			callbackEvents++
			_, _, _ = deploymentID, from, to
		},
	})

	// Use a no-op provider to exercise deployer full lifecycle without external side effects.
	mockProvider := &demoDeploymentProvider{}
	deployer.RegisterProvider(deployment.TargetLocal, mockProvider)

	deployed, err := deployer.Deploy(ctx, deployment.DeployOptions{
		Name:     "my-agent",
		AgentID:  "agent_001",
		Target:   deployment.TargetLocal,
		Replicas: 2,
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
		Metadata: map[string]string{"env": "demo"},
	})
	if err != nil {
		fmt.Printf("   Deploy error: %v\n\n", err)
		return
	}

	_ = deployer.Update(ctx, deployed.ID, deployment.DeploymentConfig{
		Image: "myregistry/agent:v1.1",
		Port:  9090,
	})
	_ = deployer.Scale(ctx, deployed.ID, 3)
	_, _ = deployer.GetDeployment(deployed.ID)
	_ = deployer.ListDeployments()
	_ = deployer.GetDeploymentsByAgent("agent_001")
	manifest, _ := deployer.ExportManifest(deployed.ID)
	_ = deployer.Delete(ctx, deployed.ID)

	// Touch local/docker providers to mark provider implementations reachable.
	localProvider := deployment.NewLocalProvider(logger)
	localDep := &deployment.Deployment{
		ID:   "local_dep_demo",
		Name: "local-agent",
		Config: deployment.DeploymentConfig{
			Image: "/bin/echo",
			Port:  0,
			Environment: map[string]string{
				"DEMO_ENV": "1",
			},
		},
	}
	_ = localProvider.Deploy(ctx, localDep)
	_, _ = localProvider.GetStatus(ctx, localDep.ID)
	_, _ = localProvider.GetLogs(ctx, localDep.ID, 10)
	_ = localProvider.Scale(ctx, localDep.ID, 2)
	_ = localProvider.Update(ctx, localDep)
	_ = localProvider.Delete(ctx, localDep.ID)

	dockerProvider := deployment.NewDockerProvider(logger)
	dockerDep := &deployment.Deployment{
		ID:   "docker_dep_demo",
		Name: "docker-agent",
		Config: deployment.DeploymentConfig{
			Image: "busybox:latest",
			Port:  8080,
		},
	}
	_ = dockerProvider.Deploy(ctx, dockerDep)
	_ = dockerProvider.Update(ctx, dockerDep)
	_, _ = dockerProvider.GetStatus(ctx, dockerDep.ID)
	_, _ = dockerProvider.GetLogs(ctx, dockerDep.ID, 5)
	_ = dockerProvider.Scale(ctx, dockerDep.ID, 2)
	_ = dockerProvider.Delete(ctx, dockerDep.ID)

	manifestPreview, _ := json.MarshalIndent(map[string]any{
		"id":         deployed.ID,
		"status":     deployed.Status,
		"callbacks":  callbackEvents,
		"mock_calls": len(mockProvider.calls),
	}, "   ", "  ")
	fmt.Printf("   K8s Deployment Manifest:\n   %s\n", string(manifest))
	fmt.Printf("   Deployment Summary:\n   %s\n\n", string(manifestPreview))
}

func demoK8sOperator(ctx context.Context, logger *zap.Logger) {
	fmt.Println("4.5 Kubernetes Operator")

	cfg := k8s.DefaultOperatorConfig()
	cfg.Namespace = "demo"
	cfg.ReconcileInterval = 20 * time.Millisecond

	op := k8s.NewAgentOperator(cfg, logger)
	provider := k8s.NewInMemoryInstanceProvider(logger)
	op.SetInstanceProvider(provider)
	op.SetReconcileCallback(func(agent *k8s.AgentCRD) error { return nil })
	op.SetScaleCallback(func(agent *k8s.AgentCRD, replicas int32) error { return nil })
	op.SetHealthCheckCallback(func(agent *k8s.AgentCRD) (bool, error) { return true, nil })
	_ = op.Start(ctx)
	defer op.Stop()

	agentCRD := &k8s.AgentCRD{
		APIVersion: "agentflow.io/v1",
		Kind:       "Agent",
		Metadata: k8s.ObjectMeta{
			Name:      "demo-agent",
			Namespace: "demo",
			Labels:    map[string]string{"app": "agentflow"},
		},
		Spec: k8s.AgentSpec{
			AgentType: "assistant",
			Replicas:  1,
			Model: k8s.ModelSpec{
				Provider: "openai",
				Model:    "gpt-4o-mini",
			},
			Scaling: k8s.ScalingSpec{
				Enabled:     true,
				MinReplicas: 1,
				MaxReplicas: 2,
				TargetMetrics: []k8s.TargetMetric{
					{Name: "cpu", Type: "cpu", TargetValue: 70},
				},
			},
			HealthCheck: k8s.HealthCheckSpec{
				Enabled:          true,
				Interval:         20 * time.Millisecond,
				Timeout:          10 * time.Millisecond,
				FailureThreshold: 3,
			},
		},
	}
	_ = op.RegisterAgent(agentCRD)
	_ = op.GetAgent("demo", "demo-agent")
	_ = op.ListAgents()

	time.Sleep(80 * time.Millisecond)
	instances := op.GetInstances("demo", "demo-agent")
	if len(instances) > 0 {
		op.UpdateInstanceMetrics(instances[0].ID, k8s.InstanceMetrics{
			RequestsTotal:     10,
			RequestsPerSecond: 2.5,
			AverageLatency:    40 * time.Millisecond,
			CPUUsage:          0.35,
			MemoryUsage:       0.42,
		})
	}

	crdJSON, _ := op.ExportCRD("demo", "demo-agent")
	_ = op.ImportCRD(crdJSON)
	_ = op.GetMetrics()

	// Direct provider calls for full in-memory provider reachability.
	inst, _ := provider.CreateInstance(ctx, agentCRD)
	if inst != nil {
		_, _ = provider.GetInstanceStatus(ctx, inst.ID)
		_, _ = provider.ListInstances(ctx, inst.Namespace, inst.AgentName)
		_ = provider.DeleteInstance(ctx, inst.ID)
	}

	_ = op.UnregisterAgent("demo", "demo-agent")
	fmt.Println("   K8s operator paths wired")
	fmt.Println()
}

type demoDeploymentProvider struct {
	calls []string
}

func (p *demoDeploymentProvider) Deploy(ctx context.Context, d *deployment.Deployment) error {
	p.calls = append(p.calls, "deploy")
	d.Endpoint = "http://127.0.0.1:18080"
	return nil
}

func (p *demoDeploymentProvider) Update(ctx context.Context, d *deployment.Deployment) error {
	p.calls = append(p.calls, "update")
	return nil
}

func (p *demoDeploymentProvider) Delete(ctx context.Context, deploymentID string) error {
	p.calls = append(p.calls, "delete")
	return nil
}

func (p *demoDeploymentProvider) GetStatus(ctx context.Context, deploymentID string) (*deployment.Deployment, error) {
	p.calls = append(p.calls, "status")
	return &deployment.Deployment{ID: deploymentID, Status: deployment.StatusRunning}, nil
}

func (p *demoDeploymentProvider) Scale(ctx context.Context, deploymentID string, replicas int) error {
	p.calls = append(p.calls, "scale")
	return nil
}

func (p *demoDeploymentProvider) GetLogs(ctx context.Context, deploymentID string, lines int) ([]string, error) {
	p.calls = append(p.calls, "logs")
	return []string{"demo log"}, nil
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
	fmt.Printf("   Diff v1->v3: changed=%v\n", diff.ChangedNodes)

	// Create + resume + rollback with real manager methods
	graph := workflow.NewDAGGraph()
	graph.AddNode(&workflow.DAGNode{
		ID:   "start",
		Type: workflow.NodeTypeAction,
		Step: workflow.NewFuncStep("start", func(ctx context.Context, input any) (any, error) {
			return "ok", nil
		}),
	})
	graph.SetEntry("start")
	executor := workflow.NewDAGExecutor(nil, logger)

	created, err := manager.CreateCheckpoint(ctx, executor, graph, "thread_demo_rt", map[string]any{"demo": true})
	if err != nil {
		fmt.Printf("   CreateCheckpoint error: %v\n\n", err)
		return
	}

	resumed, err := manager.ResumeFromCheckpoint(ctx, created.ID, graph)
	if err != nil {
		fmt.Printf("   ResumeFromCheckpoint error: %v\n\n", err)
		return
	}
	_, resumedOK := resumed.GetNodeResult("start")

	rolled, err := manager.Rollback(ctx, "thread_demo_rt", 1)
	if err != nil {
		fmt.Printf("   Rollback error: %v\n\n", err)
		return
	}

	fmt.Printf("   Runtime checkpoint: id=%s resumed_start=%v rollback_version=%d\n\n", created.ID, resumedOK, rolled.Version)
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
			{ID: "code", Type: workflow.VNodeCode, Label: "Code Step", Position: workflow.Position{X: 500, Y: 100}},
			{ID: "end", Type: workflow.VNodeEnd, Label: "End", Position: workflow.Position{X: 700, Y: 100}},
		},
		Edges: []workflow.VisualEdge{
			{ID: "e1", Source: "start", Target: "llm"},
			{ID: "e2", Source: "llm", Target: "code"},
			{ID: "e3", Source: "code", Target: "end"},
		},
	}

	// Validate
	if err := vw.Validate(); err != nil {
		fmt.Printf("   Validation error: %v\n", err)
		return
	}

	// Build executable DAG
	builder := workflow.NewVisualBuilder()
	builder.RegisterStep("custom-tool", workflow.NewFuncStep("custom-tool", func(ctx context.Context, input any) (any, error) {
		return map[string]any{"ok": true, "source": "visual-builder"}, nil
	}))

	toolWorkflow := &workflow.VisualWorkflow{
		ID:   "vw_tool",
		Name: "Tool Workflow",
		Nodes: []workflow.VisualNode{
			{ID: "start", Type: workflow.VNodeStart, Label: "Start"},
			{ID: "tool", Type: workflow.VNodeTool, Label: "Tool", Config: workflow.NodeConfig{ToolName: "custom-tool"}},
		},
		Edges: []workflow.VisualEdge{
			{ID: "te1", Source: "start", Target: "tool"},
		},
	}
	if _, err := builder.Build(toolWorkflow); err != nil {
		fmt.Printf("   Build tool workflow error: %v\n", err)
	}

	serialized, err := vw.Export()
	if err != nil {
		fmt.Printf("   Export error: %v\n", err)
		return
	}
	imported, err := workflow.Import(serialized)
	if err != nil {
		fmt.Printf("   Import error: %v\n", err)
		return
	}

	dag, err := builder.Build(vw)
	if err != nil {
		fmt.Printf("   Build error: %v\n", err)
		return
	}

	fmt.Printf("   Built DAG workflow: %s (imported=%s)\n", dag.Name(), imported.Name)
	fmt.Printf("   Nodes: %d, Entry: %s\n", len(dag.Graph().Nodes()), dag.Graph().GetEntry())
	executor := workflow.NewDAGExecutor(nil, logger)
	out, execErr := executor.Execute(ctx, dag.Graph(), map[string]any{"message": "demo"})
	fmt.Printf("   Execute result type: %T, err: %v\n", out, execErr)
}

func demoDSLParserAndObservability(ctx context.Context) {
	fmt.Println("7. DSL Parser + Workflow Observability Context")

	const dslYAML = `
version: "1.0"
name: "dsl-observability-demo"
description: "ParseFile + RegisterCondition + stream/observe context demo"
workflow:
  entry: start
  nodes:
    - id: start
      type: action
      step_def:
        type: passthrough
      next: [check]
    - id: check
      type: condition
      condition: always_true
      on_true: [done]
      on_false: [fallback]
    - id: done
      type: action
      step_def:
        type: passthrough
    - id: fallback
      type: action
      step_def:
        type: passthrough
`

	tmpFile, err := os.CreateTemp("", "agentflow-dsl-*.yaml")
	if err != nil {
		fmt.Printf("   CreateTemp error: %v\n", err)
		return
	}
	defer os.Remove(tmpFile.Name())
	if _, err = tmpFile.WriteString(dslYAML); err != nil {
		fmt.Printf("   Write DSL error: %v\n", err)
		_ = tmpFile.Close()
		return
	}
	_ = tmpFile.Close()

	parser := dsl.NewParser()
	parser.RegisterCondition("always_true", func(ctx context.Context, input any) (bool, error) {
		return true, nil
	})

	wf, err := parser.ParseFile(tmpFile.Name())
	if err != nil {
		fmt.Printf("   ParseFile error: %v\n", err)
		return
	}

	streamEvents := 0
	nodeEvents := 0
	obsCtx := workflow.WithWorkflowStreamEmitter(ctx, func(event workflow.WorkflowStreamEvent) {
		streamEvents++
	})
	obsCtx = workflowobs.WithNodeEventEmitter(obsCtx, func(event workflowobs.NodeEvent) {
		nodeEvents++
	})

	result, err := wf.Execute(obsCtx, map[string]any{"source": "dsl-demo"})
	fmt.Printf("   Result type: %T, err: %v, stream_events=%d, node_events=%d\n", result, err, streamEvents, nodeEvents)
}

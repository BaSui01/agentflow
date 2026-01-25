package workflow

import (
	"context"
	"errors"
	"testing"
)

func TestChainWorkflow(t *testing.T) {
	// 创建测试步骤
	step1 := NewFuncStep("step1", func(ctx context.Context, input interface{}) (interface{}, error) {
		str := input.(string)
		return str + " -> step1", nil
	})

	step2 := NewFuncStep("step2", func(ctx context.Context, input interface{}) (interface{}, error) {
		str := input.(string)
		return str + " -> step2", nil
	})

	step3 := NewFuncStep("step3", func(ctx context.Context, input interface{}) (interface{}, error) {
		str := input.(string)
		return str + " -> step3", nil
	})

	// 创建工作流
	workflow := NewChainWorkflow("test-chain", "Test chain workflow", step1, step2, step3)

	// 执行
	ctx := context.Background()
	result, err := workflow.Execute(ctx, "start")
	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}

	expected := "start -> step1 -> step2 -> step3"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestChainWorkflow_StepError(t *testing.T) {
	step1 := NewFuncStep("step1", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "step1", nil
	})

	step2 := NewFuncStep("step2", func(ctx context.Context, input interface{}) (interface{}, error) {
		return nil, errors.New("step2 failed")
	})

	step3 := NewFuncStep("step3", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "step3", nil
	})

	workflow := NewChainWorkflow("test-chain-error", "Test chain with error", step1, step2, step3)

	ctx := context.Background()
	_, err := workflow.Execute(ctx, "start")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "step 2 (step2) failed: step2 failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChainWorkflow_ContextCancellation(t *testing.T) {
	step1 := NewFuncStep("step1", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "step1", nil
	})

	step2 := NewFuncStep("step2", func(ctx context.Context, input interface{}) (interface{}, error) {
		// 模拟长时间运行
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	workflow := NewChainWorkflow("test-chain-cancel", "Test chain with cancellation", step1, step2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := workflow.Execute(ctx, "start")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRoutingWorkflow(t *testing.T) {
	// 创建路由器
	router := NewFuncRouter(func(ctx context.Context, input interface{}) (string, error) {
		str := input.(string)
		if str == "route-a" {
			return "handler-a", nil
		}
		return "handler-b", nil
	})

	// 创建处理器
	handlerA := NewFuncHandler("handler-a", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "handled by A", nil
	})

	handlerB := NewFuncHandler("handler-b", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "handled by B", nil
	})

	// 创建工作流
	workflow := NewRoutingWorkflow("test-routing", "Test routing workflow", router)
	workflow.RegisterHandler("handler-a", handlerA)
	workflow.RegisterHandler("handler-b", handlerB)

	// 测试路由到 A
	ctx := context.Background()
	result, err := workflow.Execute(ctx, "route-a")
	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}
	if result != "handled by A" {
		t.Errorf("expected 'handled by A', got %q", result)
	}

	// 测试路由到 B
	result, err = workflow.Execute(ctx, "route-b")
	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}
	if result != "handled by B" {
		t.Errorf("expected 'handled by B', got %q", result)
	}
}

func TestRoutingWorkflow_NoHandler(t *testing.T) {
	router := NewFuncRouter(func(ctx context.Context, input interface{}) (string, error) {
		return "unknown-route", nil
	})

	workflow := NewRoutingWorkflow("test-routing-no-handler", "Test routing with no handler", router)

	ctx := context.Background()
	_, err := workflow.Execute(ctx, "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRoutingWorkflow_DefaultRoute(t *testing.T) {
	router := NewFuncRouter(func(ctx context.Context, input interface{}) (string, error) {
		return "unknown-route", nil
	})

	defaultHandler := NewFuncHandler("default", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "default handler", nil
	})

	workflow := NewRoutingWorkflow("test-routing-default", "Test routing with default", router)
	workflow.RegisterHandler("default", defaultHandler)
	workflow.SetDefaultRoute("default")

	ctx := context.Background()
	result, err := workflow.Execute(ctx, "test")
	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}
	if result != "default handler" {
		t.Errorf("expected 'default handler', got %q", result)
	}
}

func TestParallelWorkflow(t *testing.T) {
	// 创建任务
	task1 := NewFuncTask("task1", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result1", nil
	})

	task2 := NewFuncTask("task2", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result2", nil
	})

	task3 := NewFuncTask("task3", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result3", nil
	})

	// 创建聚合器
	aggregator := NewFuncAggregator(func(ctx context.Context, results []TaskResult) (interface{}, error) {
		combined := ""
		for _, r := range results {
			combined += r.Result.(string) + " "
		}
		return combined, nil
	})

	// 创建工作流
	workflow := NewParallelWorkflow("test-parallel", "Test parallel workflow", aggregator, task1, task2, task3)

	// 执行
	ctx := context.Background()
	result, err := workflow.Execute(ctx, "input")
	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}

	// 验证结果包含所有任务的输出
	resultStr := result.(string)
	if !contains(resultStr, "result1") || !contains(resultStr, "result2") || !contains(resultStr, "result3") {
		t.Errorf("expected all results, got %q", resultStr)
	}
}

func TestParallelWorkflow_TaskError(t *testing.T) {
	task1 := NewFuncTask("task1", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result1", nil
	})

	task2 := NewFuncTask("task2", func(ctx context.Context, input interface{}) (interface{}, error) {
		return nil, errors.New("task2 failed")
	})

	aggregator := NewFuncAggregator(func(ctx context.Context, results []TaskResult) (interface{}, error) {
		return "aggregated", nil
	})

	workflow := NewParallelWorkflow("test-parallel-error", "Test parallel with error", aggregator, task1, task2)

	ctx := context.Background()
	_, err := workflow.Execute(ctx, "input")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestParallelWorkflow_NoAggregator(t *testing.T) {
	task1 := NewFuncTask("task1", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result1", nil
	})

	task2 := NewFuncTask("task2", func(ctx context.Context, input interface{}) (interface{}, error) {
		return "result2", nil
	})

	// 没有聚合器
	workflow := NewParallelWorkflow("test-parallel-no-agg", "Test parallel without aggregator", nil, task1, task2)

	ctx := context.Background()
	result, err := workflow.Execute(ctx, "input")
	if err != nil {
		t.Fatalf("workflow execution failed: %v", err)
	}

	// 应该返回原始结果数组
	results := result.([]TaskResult)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

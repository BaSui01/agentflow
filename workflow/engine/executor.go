package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/workflow/core"
)

// ExecutionMode 工作流执行模式。
type ExecutionMode string

const (
	ModeSequential ExecutionMode = "sequential"
	ModeParallel   ExecutionMode = "parallel"
	ModeRouting    ExecutionMode = "routing"
)

// StepRunner 执行单个步骤的回调。
type StepRunner func(ctx context.Context, step core.StepProtocol, input core.StepInput) (core.StepOutput, error)

// ScheduleStrategy 调度策略接口（Strategy Pattern）。
type ScheduleStrategy interface {
	Schedule(ctx context.Context, nodes []*ExecutionNode, runner StepRunner) (*ExecutionResult, error)
}

// ExecutionNode 执行节点，包装步骤与依赖关系。
type ExecutionNode struct {
	ID           string
	Step         core.StepProtocol
	Dependencies []string // 依赖的节点 ID
	Input        core.StepInput
}

// ExecutionResult 执行结果。
type ExecutionResult struct {
	Outputs map[string]core.StepOutput // nodeID -> output
	Errors  map[string]error           // nodeID -> error (if any)
}

// Executor 唯一执行入口。
// 对外仅暴露 Execute，策略选择在内部完成。
type Executor struct {
	strategies map[ExecutionMode]ScheduleStrategy
	mu         sync.RWMutex
}

// NewExecutor 创建执行器，注册内置策略。
func NewExecutor() *Executor {
	e := &Executor{
		strategies: make(map[ExecutionMode]ScheduleStrategy),
	}
	e.RegisterStrategy(ModeSequential, &SequentialStrategy{})
	e.RegisterStrategy(ModeParallel, &ParallelStrategy{})
	e.RegisterStrategy(ModeRouting, &RoutingStrategy{})
	return e
}

// RegisterStrategy 注册自定义策略。
func (e *Executor) RegisterStrategy(mode ExecutionMode, strategy ScheduleStrategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies[mode] = strategy
}

// Execute 执行工作流，根据 mode 选择策略。
func (e *Executor) Execute(ctx context.Context, mode ExecutionMode, nodes []*ExecutionNode, runner StepRunner) (*ExecutionResult, error) {
	if runner == nil {
		runner = DefaultStepRunner
	}

	e.mu.RLock()
	strategy, ok := e.strategies[mode]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported execution mode: %s", mode)
	}

	// Keep builtin strategy dispatch explicit so runtime integration
	// reaches concrete schedulers without relying only on interface calls.
	switch s := strategy.(type) {
	case *SequentialStrategy:
		return s.Schedule(ctx, nodes, runner)
	case *ParallelStrategy:
		return s.Schedule(ctx, nodes, runner)
	case *RoutingStrategy:
		return s.Schedule(ctx, nodes, runner)
	default:
		return strategy.Schedule(ctx, nodes, runner)
	}
}

// SequentialStrategy 按序执行（Chain 模式）。
type SequentialStrategy struct{}

func (s *SequentialStrategy) Schedule(ctx context.Context, nodes []*ExecutionNode, runner StepRunner) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Outputs: make(map[string]core.StepOutput),
		Errors:  make(map[string]error),
	}

	var lastOutput core.StepOutput
	for _, node := range nodes {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		// 将上一步输出合并到当前输入
		input := node.Input
		if input.Data == nil {
			input.Data = make(map[string]any)
		}
		for k, v := range lastOutput.Data {
			if _, exists := input.Data[k]; !exists {
				input.Data[k] = v
			}
		}

		output, err := runner(ctx, node.Step, input)
		if err != nil {
			result.Errors[node.ID] = err
			return result, fmt.Errorf("node %s failed: %w", node.ID, err)
		}

		result.Outputs[node.ID] = output
		lastOutput = output
	}

	return result, nil
}

// ParallelStrategy 无依赖步骤并发执行。
type ParallelStrategy struct{}

func (s *ParallelStrategy) Schedule(ctx context.Context, nodes []*ExecutionNode, runner StepRunner) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Outputs: make(map[string]core.StepOutput),
		Errors:  make(map[string]error),
	}

	if len(nodes) == 0 {
		return result, nil
	}

	type nodeResult struct {
		id     string
		output core.StepOutput
		err    error
	}

	ch := make(chan nodeResult, len(nodes))
	var wg sync.WaitGroup

	for _, node := range nodes {
		wg.Add(1)
		go func(n *ExecutionNode) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					ch <- nodeResult{
						id:  n.ID,
						err: fmt.Errorf("node panicked: %v", r),
					}
				}
			}()
			// T-006: 入口检查 ctx.Done()，避免 goroutine 在已取消时继续执行
			select {
			case <-ctx.Done():
				ch <- nodeResult{id: n.ID, err: ctx.Err()}
				return
			default:
			}
			output, err := runner(ctx, n.Step, n.Input)
			ch <- nodeResult{id: n.ID, output: output, err: err}
		}(node)
	}

	wg.Wait()
	close(ch)

	var firstErr error
	for nr := range ch {
		if nr.err != nil {
			result.Errors[nr.id] = nr.err
			if firstErr == nil {
				firstErr = fmt.Errorf("node %s failed: %w", nr.id, nr.err)
			}
		} else {
			result.Outputs[nr.id] = nr.output
		}
	}

	return result, firstErr
}

// RouteSelector 路由选择函数，根据输入选择要执行的节点。
type RouteSelector func(ctx context.Context, input core.StepInput, nodes []*ExecutionNode) (*ExecutionNode, error)

// RoutingStrategy 条件分支选择。
type RoutingStrategy struct {
	Selector RouteSelector
}

func (s *RoutingStrategy) Schedule(ctx context.Context, nodes []*ExecutionNode, runner StepRunner) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Outputs: make(map[string]core.StepOutput),
		Errors:  make(map[string]error),
	}

	if len(nodes) == 0 {
		return result, nil
	}

	// 如果没有 selector，执行第一个节点
	var selected *ExecutionNode
	if s.Selector != nil {
		var input core.StepInput
		if len(nodes) > 0 {
			input = nodes[0].Input
		}
		var err error
		selected, err = s.Selector(ctx, input, nodes)
		if err != nil {
			return result, fmt.Errorf("route selection failed: %w", err)
		}
	} else {
		selected = nodes[0]
	}

	if selected == nil {
		return result, fmt.Errorf("no route selected")
	}

	output, err := runner(ctx, selected.Step, selected.Input)
	if err != nil {
		result.Errors[selected.ID] = err
		return result, fmt.Errorf("node %s failed: %w", selected.ID, err)
	}

	result.Outputs[selected.ID] = output
	return result, nil
}

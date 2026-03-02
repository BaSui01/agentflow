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
	ModeDAG        ExecutionMode = "dag"
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
	e.strategies[ModeSequential] = &SequentialStrategy{}
	e.strategies[ModeParallel] = &ParallelStrategy{}
	e.strategies[ModeDAG] = &DAGStrategy{}
	e.strategies[ModeRouting] = &RoutingStrategy{}
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
	e.mu.RLock()
	strategy, ok := e.strategies[mode]
	e.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported execution mode: %s", mode)
	}

	return strategy.Schedule(ctx, nodes, runner)
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

// DAGStrategy 拓扑排序 + ready queue 并发。
type DAGStrategy struct{}

func (s *DAGStrategy) Schedule(ctx context.Context, nodes []*ExecutionNode, runner StepRunner) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Outputs: make(map[string]core.StepOutput),
		Errors:  make(map[string]error),
	}

	if len(nodes) == 0 {
		return result, nil
	}

	// 构建依赖图
	nodeMap := make(map[string]*ExecutionNode, len(nodes))
	inDegree := make(map[string]int, len(nodes))
	dependents := make(map[string][]string) // nodeID -> 依赖它的节点

	for _, n := range nodes {
		nodeMap[n.ID] = n
		inDegree[n.ID] = len(n.Dependencies)
		for _, dep := range n.Dependencies {
			dependents[dep] = append(dependents[dep], n.ID)
		}
	}

	// 初始 ready queue
	var ready []*ExecutionNode
	for _, n := range nodes {
		if inDegree[n.ID] == 0 {
			ready = append(ready, n)
		}
	}

	var mu sync.Mutex
	completed := 0
	total := len(nodes)

	for completed < total {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if len(ready) == 0 {
			return result, fmt.Errorf("DAG deadlock: no ready nodes but %d/%d completed", completed, total)
		}

		// 并发执行所有 ready 节点
		batch := ready
		ready = nil

		type nodeResult struct {
			id     string
			output core.StepOutput
			err    error
		}

		ch := make(chan nodeResult, len(batch))
		var wg sync.WaitGroup

		for _, n := range batch {
			wg.Add(1)
			go func(node *ExecutionNode) {
				defer wg.Done()
				// 合并依赖节点的输出到输入
				input := node.Input
				if input.Data == nil {
					input.Data = make(map[string]any)
				}
				mu.Lock()
				for _, dep := range node.Dependencies {
					if out, ok := result.Outputs[dep]; ok {
						for k, v := range out.Data {
							if _, exists := input.Data[k]; !exists {
								input.Data[k] = v
							}
						}
					}
				}
				mu.Unlock()

				output, err := runner(ctx, node.Step, input)
				ch <- nodeResult{id: node.ID, output: output, err: err}
			}(n)
		}

		wg.Wait()
		close(ch)

		for nr := range ch {
			mu.Lock()
			if nr.err != nil {
				result.Errors[nr.id] = nr.err
				mu.Unlock()
				return result, fmt.Errorf("node %s failed: %w", nr.id, nr.err)
			}
			result.Outputs[nr.id] = nr.output
			completed++

			// 更新依赖计数，找出新的 ready 节点
			for _, depID := range dependents[nr.id] {
				inDegree[depID]--
				if inDegree[depID] == 0 {
					ready = append(ready, nodeMap[depID])
				}
			}
			mu.Unlock()
		}
	}

	return result, nil
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

package workflow

import (
	"context"
	"fmt"
	"sync"
)

// Task 并行任务接口
type Task interface {
	// Execute 执行任务
	Execute(ctx context.Context, input any) (any, error)
	// Name 返回任务名称
	Name() string
}

// TaskFunc 任务函数类型
type TaskFunc func(ctx context.Context, input any) (any, error)

// FuncTask 函数任务
type FuncTask struct {
	name string
	fn   TaskFunc
}

// NewFuncTask 创建函数任务
func NewFuncTask(name string, fn TaskFunc) *FuncTask {
	return &FuncTask{
		name: name,
		fn:   fn,
	}
}

func (t *FuncTask) Execute(ctx context.Context, input any) (any, error) {
	return t.fn(ctx, input)
}

func (t *FuncTask) Name() string {
	return t.name
}

// TaskResult 任务结果
type TaskResult struct {
	TaskName string
	Result   any
	Error    error
}

// Aggregator 聚合器接口
// 将多个任务的结果聚合为最终输出
type Aggregator interface {
	// Aggregate 聚合结果
	Aggregate(ctx context.Context, results []TaskResult) (any, error)
}

// AggregatorFunc 聚合器函数类型
type AggregatorFunc func(ctx context.Context, results []TaskResult) (any, error)

// FuncAggregator 函数聚合器
type FuncAggregator struct {
	fn AggregatorFunc
}

// NewFuncAggregator 创建函数聚合器
func NewFuncAggregator(fn AggregatorFunc) *FuncAggregator {
	return &FuncAggregator{fn: fn}
}

func (a *FuncAggregator) Aggregate(ctx context.Context, results []TaskResult) (any, error) {
	return a.fn(ctx, results)
}

// ParallelWorkflow 并行工作流
// 将任务分割为多个子任务并行执行，然后聚合结果
type ParallelWorkflow struct {
	name        string
	description string
	tasks       []Task
	aggregator  Aggregator
}

// NewParallelWorkflow 创建并行工作流
func NewParallelWorkflow(name, description string, aggregator Aggregator, tasks ...Task) *ParallelWorkflow {
	return &ParallelWorkflow{
		name:        name,
		description: description,
		tasks:       tasks,
		aggregator:  aggregator,
	}
}

// Execute 执行并行工作流
// 1. 并行执行所有任务
// 2. 收集所有结果
// 3. 使用聚合器聚合结果
func (w *ParallelWorkflow) Execute(ctx context.Context, input any) (any, error) {
	if len(w.tasks) == 0 {
		return nil, fmt.Errorf("no tasks to execute")
	}

	// 创建结果通道
	resultCh := make(chan TaskResult, len(w.tasks))
	var wg sync.WaitGroup

	// 启动所有任务
	for _, task := range w.tasks {
		wg.Add(1)
		go func(t Task) {
			defer wg.Done()

			result, err := t.Execute(ctx, input)
			resultCh <- TaskResult{
				TaskName: t.Name(),
				Result:   result,
				Error:    err,
			}
		}(task)
	}

	// 等待所有任务完成
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 收集结果
	results := make([]TaskResult, 0, len(w.tasks))
	for result := range resultCh {
		results = append(results, result)
	}

	// 检查是否有错误
	var errors []error
	for _, result := range results {
		if result.Error != nil {
			errors = append(errors, fmt.Errorf("task %s failed: %w", result.TaskName, result.Error))
		}
	}

	if len(errors) > 0 {
		return nil, fmt.Errorf("parallel execution failed with %d errors: %v", len(errors), errors)
	}

	// 聚合结果
	if w.aggregator == nil {
		// 如果没有聚合器，直接返回所有结果
		return results, nil
	}

	aggregated, err := w.aggregator.Aggregate(ctx, results)
	if err != nil {
		return nil, fmt.Errorf("aggregation failed: %w", err)
	}

	return aggregated, nil
}

func (w *ParallelWorkflow) Name() string {
	return w.name
}

func (w *ParallelWorkflow) Description() string {
	return w.description
}

// AddTask 添加任务
func (w *ParallelWorkflow) AddTask(task Task) {
	w.tasks = append(w.tasks, task)
}

// Tasks 返回所有任务
func (w *ParallelWorkflow) Tasks() []Task {
	return w.tasks
}

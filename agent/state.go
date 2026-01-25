package agent

import "fmt"

// State 定义 Agent 生命周期状态
type State string

const (
	StateInit      State = "init"      // 初始化中
	StateReady     State = "ready"     // 就绪，可执行
	StateRunning   State = "running"   // 执行中
	StatePaused    State = "paused"    // 暂停（等待人工/外部输入）
	StateCompleted State = "completed" // 已完成
	StateFailed    State = "failed"    // 失败
)

// validTransitions 定义合法的状态转换
var validTransitions = map[State][]State{
	StateInit:      {StateReady, StateFailed},
	StateReady:     {StateRunning, StateFailed},
	StateRunning:   {StateReady, StatePaused, StateCompleted, StateFailed}, // 支持中断后重试
	StatePaused:    {StateRunning, StateCompleted, StateFailed},            // 支持暂停后直接结束
	StateCompleted: {StateReady},                                           // 支持重新调度
	StateFailed:    {StateReady, StateInit},                                // 支持重试或重置
}

// CanTransition 检查状态转换是否合法
func CanTransition(from, to State) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

// ErrInvalidTransition 非法状态转换错误
type ErrInvalidTransition struct {
	From State
	To   State
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("invalid state transition: %s -> %s", e.From, e.To)
}

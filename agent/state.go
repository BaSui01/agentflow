package agent

import "fmt"

// State 定义 Agent 生命周期状态
type State string

const (
	StateInit      State = "init"      // Initializing
	StateReady     State = "ready"     // Ready to execute
	StateRunning   State = "running"   // Executing
	StatePaused    State = "paused"    // Paused (waiting for human/external input)
	StateCompleted State = "completed" // Completed
	StateFailed    State = "failed"    // Failed
)

// validTransitions 定义合法的状态转换
var validTransitions = map[State][]State{
	StateInit:      {StateReady, StateFailed},
	StateReady:     {StateRunning, StateFailed},
	StateRunning:   {StateReady, StatePaused, StateCompleted, StateFailed}, // Support retry after interruption
	StatePaused:    {StateRunning, StateCompleted, StateFailed},            // Support direct completion after pause
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

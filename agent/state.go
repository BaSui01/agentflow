package agent

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

// Err Invalid Transition 现在定义在错误中. go
// 此注释用于向后兼容

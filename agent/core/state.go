package core

// State defines the agent lifecycle state contract.
type State string

const (
	StateInit      State = "init"
	StateReady     State = "ready"
	StateRunning   State = "running"
	StatePaused    State = "paused"
	StateCompleted State = "completed"
	StateFailed    State = "failed"
)

var validTransitions = map[State][]State{
	StateInit:      {StateReady, StateFailed},
	StateReady:     {StateRunning, StateFailed},
	StateRunning:   {StateReady, StatePaused, StateCompleted, StateFailed},
	StatePaused:    {StateRunning, StateCompleted, StateFailed},
	StateCompleted: {StateReady, StateInit},
	StateFailed:    {StateReady, StateInit},
}

// CanTransition returns whether transition from -> to is legal.
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

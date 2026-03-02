package gateway

import (
	"fmt"
	"sync"
)

// RequestState 定义统一请求执行状态。
type RequestState string

const (
	StatePlanned   RequestState = "planned"
	StateValidated RequestState = "validated"
	StateRouted    RequestState = "routed"
	StateExecuting RequestState = "executing"
	StateStreaming RequestState = "streaming"
	StateCompleted RequestState = "completed"
	StateFailed    RequestState = "failed"
	StateRetried   RequestState = "retried"
	StateDegraded  RequestState = "degraded"
)

var validTransitions = map[RequestState]map[RequestState]struct{}{
	StatePlanned:   {StateValidated: {}},
	StateValidated: {StateRouted: {}},
	StateRouted:    {StateExecuting: {}},
	StateExecuting: {StateStreaming: {}, StateCompleted: {}, StateFailed: {}, StateRetried: {}, StateDegraded: {}},
	StateStreaming: {StateCompleted: {}, StateFailed: {}},
	StateRetried:   {StateExecuting: {}, StateFailed: {}, StateCompleted: {}},
	StateDegraded:  {StateExecuting: {}, StateFailed: {}, StateCompleted: {}},
}

// StateMachine 维护请求状态机。
type StateMachine struct {
	mu    sync.Mutex
	state RequestState
}

// NewStateMachine 创建默认状态机。
func NewStateMachine() *StateMachine {
	return &StateMachine{state: StatePlanned}
}

// Current 返回当前状态。
func (sm *StateMachine) Current() RequestState {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	return sm.state
}

// Transition 执行状态迁移。
func (sm *StateMachine) Transition(next RequestState) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	allowed := validTransitions[sm.state]
	if _, ok := allowed[next]; !ok {
		return fmt.Errorf("invalid state transition: %s -> %s", sm.state, next)
	}
	sm.state = next
	return nil
}

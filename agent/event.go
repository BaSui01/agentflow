package agent

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// EventType 事件类型
type EventType = types.AgentEventType

const (
	EventStateChange       EventType = types.AgentEventStateChange
	EventToolCall          EventType = types.AgentEventToolCall
	EventFeedback          EventType = types.AgentEventFeedback
	EventApprovalRequested EventType = types.AgentEventApprovalRequested
	EventApprovalResponded EventType = types.AgentEventApprovalResponded
	EventSubagentCompleted EventType = types.AgentEventSubagentCompleted
	EventAgentRunStart     EventType = types.AgentEventRunStart
	EventAgentRunComplete  EventType = types.AgentEventRunComplete
	EventAgentRunError     EventType = types.AgentEventRunError
)

// subscriptionCounter 用于生成唯一订阅 ID，替代 time.Now().UnixNano() 避免并发碰撞
var subscriptionCounter int64

// Event 事件接口
type Event interface {
	Timestamp() time.Time
	Type() EventType
}

// EventHandler 事件处理器
type EventHandler func(Event)

// EventBus 定义事件总线接口
type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler) string
	Unsubscribe(subscriptionID string)
	Stop()
}

// SimpleEventBus 简单的事件总线实现
type SimpleEventBus struct {
	mu             sync.RWMutex
	handlers       map[EventType]map[string]EventHandler
	eventChannel   chan Event
	done           chan struct{}
	loopDone       chan struct{} // closed when processEvents goroutine exits
	stopOnce       sync.Once
	handlerWg      sync.WaitGroup // 跟踪正在运行的 handler goroutine，Stop() 时等待完成
	logger         *zap.Logger
	panicErrChan   chan<- error // 可选，handler panic 时写入
	panicErrChanMu sync.RWMutex
}

// NewEventBus 创建新的事件总线
func NewEventBus(logger ...*zap.Logger) EventBus {
	var l *zap.Logger
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	} else {
		panic("agent.EventBus: logger is required and cannot be nil")
	}
	bus := &SimpleEventBus{
		handlers:     make(map[EventType]map[string]EventHandler),
		eventChannel: make(chan Event, 100),
		done:         make(chan struct{}),
		loopDone:     make(chan struct{}),
		logger:       l,
	}
	go bus.processEvents()
	return bus
}

// Publish 发布事件
func (b *SimpleEventBus) Publish(event Event) {
	select {
	case b.eventChannel <- event:
	case <-b.done:
	default:
		// T-005: 通道满时丢弃事件，记录日志便于排查背压
		b.logger.Warn("event dropped: channel full",
			zap.String("event_type", string(event.Type())),
			zap.Time("timestamp", event.Timestamp()),
		)
	}
}

// Subscribe 订阅事件
func (b *SimpleEventBus) Subscribe(eventType EventType, handler EventHandler) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.handlers[eventType] == nil {
		b.handlers[eventType] = make(map[string]EventHandler)
	}

	id := fmt.Sprintf("%s-%d", eventType, atomic.AddInt64(&subscriptionCounter, 1))
	b.handlers[eventType][id] = handler
	return id
}

// SetPanicErrorChan 设置 handler panic 时写入的 error channel，供调用方消费
func (b *SimpleEventBus) SetPanicErrorChan(ch chan<- error) {
	b.panicErrChanMu.Lock()
	defer b.panicErrChanMu.Unlock()
	b.panicErrChan = ch
}

// Unsubscribe 取消订阅
func (b *SimpleEventBus) Unsubscribe(subscriptionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for eventType, handlers := range b.handlers {
		if _, ok := handlers[subscriptionID]; ok {
			delete(handlers, subscriptionID)
			if len(handlers) == 0 {
				delete(b.handlers, eventType)
			}
			return
		}
	}
}

// processEvents 处理事件
func (b *SimpleEventBus) processEvents() {
	defer close(b.loopDone)
	for {
		select {
		case event := <-b.eventChannel:
			b.dispatchEvent(event)
		case <-b.done:
			for {
				select {
				case event := <-b.eventChannel:
					b.dispatchEvent(event)
				default:
					return
				}
			}
		}
	}
}

func (b *SimpleEventBus) dispatchEvent(event Event) {
	b.mu.RLock()
	src := b.handlers[event.Type()]
	handlers := make([]EventHandler, 0, len(src))
	for _, h := range src {
		handlers = append(handlers, h)
	}
	b.mu.RUnlock()

	// Add to WaitGroup before checking done, so Stop() waits for
	// all handlers. We add the full count upfront to avoid a race
	// between individual Add(1) calls and Stop()'s Wait().
	b.handlerWg.Add(len(handlers))
	for _, handler := range handlers {
		h := handler // capture loop variable
		go func() {
			defer b.handlerWg.Done()
			defer func() {
				if r := recover(); r != nil {
					err := panicPayloadToError(r)
					b.logger.Error("event handler panicked",
						zap.Any("recover", r),
						zap.Error(err),
						zap.String("event_type", string(event.Type())),
						zap.Stack("stack"),
					)
					b.panicErrChanMu.RLock()
					ch := b.panicErrChan
					b.panicErrChanMu.RUnlock()
					if ch != nil {
						select {
						case ch <- err:
						default:
						}
					}
				}
			}()
			// 仅记录超时，不把真正的 handler 工作转移到未跟踪的内层 goroutine，
			// 否则 Stop() 可能在 handler 仍运行时提前返回。
			done := make(chan struct{})
			timer := time.AfterFunc(5*time.Second, func() {
				select {
				case <-done:
				default:
					b.logger.Warn("event handler timed out",
						zap.String("event_type", string(event.Type())),
					)
				}
			})
			defer func() {
				close(done)
				timer.Stop()
			}()
			h(event)
		}()
	}
}

// Stop 停止事件总线，等待所有 handler goroutine 完成
func (b *SimpleEventBus) Stop() {
	b.stopOnce.Do(func() {
		close(b.done)
	})
	// 先等 processEvents goroutine 退出，确保不再有新的 handlerWg.Add 调用
	<-b.loopDone
	// 再等待所有正在运行的 handler goroutine 完成，防止 goroutine 泄漏
	b.handlerWg.Wait()
}

// StateChangeEvent 状态变更事件
type StateChangeEvent struct {
	AgentID_   string
	FromState  State
	ToState    State
	Timestamp_ time.Time
}

func (e *StateChangeEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *StateChangeEvent) Type() EventType      { return EventStateChange }

// ToolCallEvent 工具调用事件
type ToolCallEvent struct {
	AgentID_            string
	RunID               string
	TraceID             string
	PromptBundleVersion string
	ToolCallID          string
	ToolName            string
	Stage               string // start/end
	Error               string
	Timestamp_          time.Time
}

func (e *ToolCallEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *ToolCallEvent) Type() EventType      { return EventToolCall }

// FeedbackEvent 反馈事件
type FeedbackEvent struct {
	AgentID_     string
	FeedbackType string
	Content      string
	Data         map[string]any
	Timestamp_   time.Time
}

func (e *FeedbackEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *FeedbackEvent) Type() EventType      { return EventFeedback }

// ====== Agent 运行级事件 ======

// AgentRunStartEvent Agent 运行开始事件。
type AgentRunStartEvent struct {
	AgentID_    string
	TraceID     string
	RunID       string
	ParentRunID string
	Timestamp_  time.Time
}

func (e *AgentRunStartEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *AgentRunStartEvent) Type() EventType      { return EventAgentRunStart }

// AgentRunCompleteEvent Agent 运行完成事件。
type AgentRunCompleteEvent struct {
	AgentID_         string
	TraceID          string
	RunID            string
	ParentRunID      string
	LatencyMs        int64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Cost             float64
	Timestamp_       time.Time
}

func (e *AgentRunCompleteEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *AgentRunCompleteEvent) Type() EventType      { return EventAgentRunComplete }

// AgentRunErrorEvent Agent 运行失败事件。
type AgentRunErrorEvent struct {
	AgentID_    string
	TraceID     string
	RunID       string
	ParentRunID string
	LatencyMs   int64
	Error       string
	Timestamp_  time.Time
}

func (e *AgentRunErrorEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *AgentRunErrorEvent) Type() EventType      { return EventAgentRunError }

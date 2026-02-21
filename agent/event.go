package agent

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// EventType 事件类型
type EventType string

const (
	EventStateChange       EventType = "state_change"
	EventToolCall          EventType = "tool_call"
	EventFeedback          EventType = "feedback"
	EventApprovalRequested EventType = "approval_requested"
	EventApprovalResponded EventType = "approval_responded"
	EventSubagentCompleted EventType = "subagent_completed"
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
	mu           sync.RWMutex
	handlers     map[EventType]map[string]EventHandler
	eventChannel chan Event
	done         chan struct{}
	stopOnce     sync.Once
	logger       *zap.Logger
}

// NewEventBus 创建新的事件总线
func NewEventBus(logger ...*zap.Logger) EventBus {
	var l *zap.Logger
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	} else {
		l = zap.NewNop()
	}
	bus := &SimpleEventBus{
		handlers:     make(map[EventType]map[string]EventHandler),
		eventChannel: make(chan Event, 100),
		done:         make(chan struct{}),
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
		// 如果通道满了，丢弃事件
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
	for {
		select {
		case event := <-b.eventChannel:
			b.mu.RLock()
			src := b.handlers[event.Type()]
			handlers := make([]EventHandler, 0, len(src))
			for _, h := range src {
				handlers = append(handlers, h)
			}
			b.mu.RUnlock()

			for _, handler := range handlers {
				h := handler // capture loop variable
				go func() {
					defer func() {
						if r := recover(); r != nil {
							b.logger.Error("event handler panicked", zap.Any("recover", r))
						}
					}()
					h(event)
				}()
			}
		case <-b.done:
			return
		}
	}
}

// Stop 停止事件总线
func (b *SimpleEventBus) Stop() {
	b.stopOnce.Do(func() {
		close(b.done)
	})
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

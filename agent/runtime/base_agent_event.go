package runtime

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/types"

	"go.uber.org/zap"
)

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

var subscriptionCounter int64

type Event interface {
	Timestamp() time.Time
	Type() EventType
}

type EventHandler func(Event)

type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler) string
	Unsubscribe(subscriptionID string)
	Stop()
}

type SimpleEventBus struct {
	mu             sync.RWMutex
	handlers       map[EventType]map[string]EventHandler
	eventChannel   chan Event
	done           chan struct{}
	loopDone       chan struct{}
	stopOnce       sync.Once
	handlerWg      sync.WaitGroup
	logger         *zap.Logger
	panicErrChan   chan<- error
	panicErrChanMu sync.RWMutex
}

func NewEventBus(logger ...*zap.Logger) (EventBus, error) {
	var l *zap.Logger
	if len(logger) > 0 && logger[0] != nil {
		l = logger[0]
	} else {
		return nil, fmt.Errorf("agent.EventBus: logger is required and cannot be nil")
	}
	bus := &SimpleEventBus{
		handlers:     make(map[EventType]map[string]EventHandler),
		eventChannel: make(chan Event, 100),
		done:         make(chan struct{}),
		loopDone:     make(chan struct{}),
		logger:       l,
	}
	go bus.processEvents()
	return bus, nil
}

func (b *SimpleEventBus) Publish(event Event) {
	select {
	case b.eventChannel <- event:
	case <-b.done:
	default:
		b.logger.Warn("event dropped: channel full",
			zap.String("event_type", string(event.Type())),
			zap.Time("timestamp", event.Timestamp()),
		)
	}
}

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

func (b *SimpleEventBus) SetPanicErrorChan(ch chan<- error) {
	b.panicErrChanMu.Lock()
	defer b.panicErrChanMu.Unlock()
	b.panicErrChan = ch
}

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
	b.handlerWg.Add(len(handlers))
	for _, handler := range handlers {
		h := handler
		go func() {
			defer b.handlerWg.Done()
			defer func() {
				if r := recover(); r != nil {
					err := agentcore.PanicPayloadToError(r)
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

func (b *SimpleEventBus) Stop() {
	b.stopOnce.Do(func() {
		close(b.done)
	})
	<-b.loopDone
	b.handlerWg.Wait()
}

type StateChangeEvent struct {
	AgentID_   string
	FromState  State
	ToState    State
	Timestamp_ time.Time
}

func (e *StateChangeEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *StateChangeEvent) Type() EventType      { return EventStateChange }

type ToolCallEvent struct {
	AgentID_            string
	RunID               string
	TraceID             string
	PromptBundleVersion string
	ToolCallID          string
	ToolName            string
	Stage               string
	Error               string
	Timestamp_          time.Time
}

func (e *ToolCallEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *ToolCallEvent) Type() EventType      { return EventToolCall }

type FeedbackEvent struct {
	AgentID_     string
	FeedbackType string
	Content      string
	Data         map[string]any
	Timestamp_   time.Time
}

func (e *FeedbackEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *FeedbackEvent) Type() EventType      { return EventFeedback }

type AgentRunStartEvent struct {
	AgentID_    string
	TraceID     string
	RunID       string
	ParentRunID string
	Timestamp_  time.Time
}

func (e *AgentRunStartEvent) Timestamp() time.Time { return e.Timestamp_ }
func (e *AgentRunStartEvent) Type() EventType      { return EventAgentRunStart }

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

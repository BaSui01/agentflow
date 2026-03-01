package types

import "time"

// EventType is a generic event type identifier.
type EventType string

// Event is the minimal event contract used by EventBus.
type Event interface {
	Timestamp() time.Time
	Type() EventType
}

// EventHandler handles a published event.
type EventHandler func(Event)

// EventBus is a cross-layer event bus contract.
type EventBus interface {
	Publish(event Event)
	Subscribe(eventType EventType, handler EventHandler) string
	Unsubscribe(subscriptionID string)
	Stop()
}

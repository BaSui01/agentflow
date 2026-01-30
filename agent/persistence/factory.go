package persistence

import (
	"fmt"
	"log"
	"os"
)

// NewMessageStore creates a new MessageStore based on the configuration
func NewMessageStore(config StoreConfig) (MessageStore, error) {
	switch config.Type {
	case StoreTypeMemory:
		return NewMemoryMessageStore(config), nil
	case StoreTypeFile:
		return NewFileMessageStore(config)
	case StoreTypeRedis:
		return NewRedisMessageStore(config)
	default:
		return nil, fmt.Errorf("unsupported message store type: %s", config.Type)
	}
}

// NewTaskStore creates a new TaskStore based on the configuration
func NewTaskStore(config StoreConfig) (TaskStore, error) {
	switch config.Type {
	case StoreTypeMemory:
		return NewMemoryTaskStore(config), nil
	case StoreTypeFile:
		return NewFileTaskStore(config)
	case StoreTypeRedis:
		return NewRedisTaskStore(config)
	default:
		return nil, fmt.Errorf("unsupported task store type: %s", config.Type)
	}
}

// MustNewMessageStore creates a new MessageStore or panics on error.
//
// WARNING: This function should ONLY be used during application initialization
// (e.g., in main() or init()). Using panic in request handlers or business logic
// is strongly discouraged. For runtime store creation, use NewMessageStore instead.
//
// Example usage:
//
//	func main() {
//	    store := persistence.MustNewMessageStore(config) // OK - initialization
//	    // ...
//	}
func MustNewMessageStore(config StoreConfig) MessageStore {
	store, err := NewMessageStore(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create message store: %v", err))
	}
	return store
}

// MustNewTaskStore creates a new TaskStore or panics on error.
//
// WARNING: This function should ONLY be used during application initialization
// (e.g., in main() or init()). Using panic in request handlers or business logic
// is strongly discouraged. For runtime store creation, use NewTaskStore instead.
//
// Example usage:
//
//	func main() {
//	    store := persistence.MustNewTaskStore(config) // OK - initialization
//	    // ...
//	}
func MustNewTaskStore(config StoreConfig) TaskStore {
	store, err := NewTaskStore(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create task store: %v", err))
	}
	return store
}

// NewMessageStoreOrExit creates a new MessageStore or exits the program on error.
// This is a safer alternative to MustNewMessageStore for CLI applications.
func NewMessageStoreOrExit(config StoreConfig) MessageStore {
	store, err := NewMessageStore(config)
	if err != nil {
		log.Printf("FATAL: failed to create message store: %v", err)
		os.Exit(1)
	}
	return store
}

// NewTaskStoreOrExit creates a new TaskStore or exits the program on error.
// This is a safer alternative to MustNewTaskStore for CLI applications.
func NewTaskStoreOrExit(config StoreConfig) TaskStore {
	store, err := NewTaskStore(config)
	if err != nil {
		log.Printf("FATAL: failed to create task store: %v", err)
		os.Exit(1)
	}
	return store
}

package persistence

import (
	"fmt"
)

// 新MessageStore 创建基于配置的新信件系统
func NewMessageStore(config StoreConfig) (MessageStore, error) {
	switch config.Type {
	case StoreTypeMemory:
		return NewMemoryMessageStore(config), nil
	default:
		return nil, fmt.Errorf("unsupported message store type: %s", config.Type)
	}
}

// NewTaskStore 创建基于配置的新任务库
func NewTaskStore(config StoreConfig) (TaskStore, error) {
	switch config.Type {
	case StoreTypeMemory:
		return NewMemoryTaskStore(config), nil
	default:
		return nil, fmt.Errorf("unsupported task store type: %s", config.Type)
	}
}

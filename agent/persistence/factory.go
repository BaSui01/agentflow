package persistence

import (
	"fmt"
)

// 新MessageStore 创建基于配置的新信件系统
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

// NewTaskStore 创建基于配置的新任务库
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

// MustNewMessageStore 保留旧命名以兼容调用点，但统一返回 error 链路，不再 panic。
func MustNewMessageStore(config StoreConfig) (MessageStore, error) {
	return NewMessageStore(config)
}

// MustNewTaskStore 保留旧命名以兼容调用点，但统一返回 error 链路，不再 panic。
func MustNewTaskStore(config StoreConfig) (TaskStore, error) {
	return NewTaskStore(config)
}

// NewMessageStoreOrExit 保留旧命名以兼容调用点，但统一返回 error 链路，不再进程退出。
func NewMessageStoreOrExit(config StoreConfig) (MessageStore, error) {
	return NewMessageStore(config)
}

// NewTaskStoreOrExit 保留旧命名以兼容调用点，但统一返回 error 链路，不再进程退出。
func NewTaskStoreOrExit(config StoreConfig) (TaskStore, error) {
	return NewTaskStore(config)
}


package persistence

import (
	"fmt"
	"os"
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

// Must NewMessageStore 创建了新信件存储器或对错误的恐慌 。
//
// 警告:此功能只应在应用程序初始化时使用
// (例如,主要()或init())。 在请求处理器或业务逻辑中使用恐慌
// 强烈劝阻。 对于运行时商店创建,取而代之的是使用"NewMessageStore".
//
// 示例用法:
//
//	func 主 () {
//	    存储 := 持久性. Must NewMessageStore(配置) // OK - 初始化
//	    // ...
//	}
func MustNewMessageStore(config StoreConfig) MessageStore {
	store, err := NewMessageStore(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create message store: %v", err))
	}
	return store
}

// MustNewTaskStore 创建了新的 TaskStore 或对错误的恐慌.
//
// 警告:此功能只应在应用程序初始化时使用
// (例如,主要()或init())。 在请求处理器或业务逻辑中使用恐慌
// 强烈劝阻。 对于运行时间商店的创建,使用"NewTaskStore"代替.
//
// 示例用法:
//
//	func 主 () {
//	    存储 := 持久性. MustNewTaskStore(配置) // OK - 初始化
//	    // ...
//	}
func MustNewTaskStore(config StoreConfig) TaskStore {
	store, err := NewTaskStore(config)
	if err != nil {
		panic(fmt.Sprintf("failed to create task store: %v", err))
	}
	return store
}

// NewMessageStore OrExit 创建了新的信件存储器,或在错误时退出程序.
// 这是用于CLI应用的MustNewMessageStore的更安全的替代品.
func NewMessageStoreOrExit(config StoreConfig) MessageStore {
	store, err := NewMessageStore(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: failed to create message store: %v\n", err)
		os.Exit(1)
	}
	return store
}

// NewTaskStoreOrExit 创建了新的 TaskStore 程序,或者在出错时退出程序.
// 这是用于CLI应用的MustNewTaskStore的更安全的替代品.
func NewTaskStoreOrExit(config StoreConfig) TaskStore {
	store, err := NewTaskStore(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: failed to create task store: %v\n", err)
		os.Exit(1)
	}
	return store
}

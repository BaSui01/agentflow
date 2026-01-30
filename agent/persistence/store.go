// Package persistence provides persistent storage interfaces and implementations
// for messages and async tasks in the AgentFlow framework.
//
// This package addresses two critical production issues:
// 1. Message loss when channels are full (MessageStore)
// 2. Task state loss on service restart (TaskStore)
//
// Supported backends:
// - Memory: For development and testing (default)
// - File: For single-node production deployments
// - Redis: For distributed production deployments
package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Common errors
var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrStoreClosed   = errors.New("store is closed")
	ErrInvalidInput  = errors.New("invalid input")
)

// StoreType represents the type of storage backend
type StoreType string

const (
	StoreTypeMemory StoreType = "memory"
	StoreTypeFile   StoreType = "file"
	StoreTypeRedis  StoreType = "redis"
)

// RetryConfig defines retry behavior for message delivery
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (default: 3)
	MaxRetries int `json:"max_retries" yaml:"max_retries"`

	// InitialBackoff is the initial backoff duration (default: 1s)
	InitialBackoff time.Duration `json:"initial_backoff" yaml:"initial_backoff"`

	// MaxBackoff is the maximum backoff duration (default: 30s)
	MaxBackoff time.Duration `json:"max_backoff" yaml:"max_backoff"`

	// BackoffMultiplier is the multiplier for exponential backoff (default: 2.0)
	BackoffMultiplier float64 `json:"backoff_multiplier" yaml:"backoff_multiplier"`
}

// DefaultRetryConfig returns the default retry configuration
// Conservative strategy: max 3 retries with exponential backoff 1s/2s/4s
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// CalculateBackoff calculates the backoff duration for a given retry attempt
func (c RetryConfig) CalculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return c.InitialBackoff
	}

	backoff := c.InitialBackoff
	for i := 0; i < attempt; i++ {
		backoff = time.Duration(float64(backoff) * c.BackoffMultiplier)
		if backoff > c.MaxBackoff {
			return c.MaxBackoff
		}
	}
	return backoff
}

// CleanupConfig defines cleanup behavior for completed tasks and old messages
type CleanupConfig struct {
	// Enabled determines if automatic cleanup is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`

	// Interval is how often cleanup runs (default: 1h)
	Interval time.Duration `json:"interval" yaml:"interval"`

	// MessageRetention is how long to keep acknowledged messages (default: 1h)
	MessageRetention time.Duration `json:"message_retention" yaml:"message_retention"`

	// TaskRetention is how long to keep completed tasks (default: 24h)
	TaskRetention time.Duration `json:"task_retention" yaml:"task_retention"`
}

// DefaultCleanupConfig returns the default cleanup configuration
func DefaultCleanupConfig() CleanupConfig {
	return CleanupConfig{
		Enabled:          true,
		Interval:         1 * time.Hour,
		MessageRetention: 1 * time.Hour,
		TaskRetention:    24 * time.Hour,
	}
}

// StoreConfig is the base configuration for all store implementations
type StoreConfig struct {
	// Type is the storage backend type
	Type StoreType `json:"type" yaml:"type"`

	// BaseDir is the base directory for file-based storage
	BaseDir string `json:"base_dir" yaml:"base_dir"`

	// Redis configuration (only used when Type is "redis")
	Redis RedisStoreConfig `json:"redis" yaml:"redis"`

	// Retry configuration
	Retry RetryConfig `json:"retry" yaml:"retry"`

	// Cleanup configuration
	Cleanup CleanupConfig `json:"cleanup" yaml:"cleanup"`
}

// RedisStoreConfig contains Redis-specific configuration
type RedisStoreConfig struct {
	// Host is the Redis server host
	Host string `json:"host" yaml:"host"`

	// Port is the Redis server port
	Port int `json:"port" yaml:"port"`

	// Password is the Redis password (optional)
	Password string `json:"password" yaml:"password"`

	// DB is the Redis database number
	DB int `json:"db" yaml:"db"`

	// PoolSize is the connection pool size
	PoolSize int `json:"pool_size" yaml:"pool_size"`

	// KeyPrefix is the prefix for all Redis keys
	KeyPrefix string `json:"key_prefix" yaml:"key_prefix"`
}

// DefaultStoreConfig returns the default store configuration
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		Type:    StoreTypeMemory,
		BaseDir: "./data/persistence",
		Redis: RedisStoreConfig{
			Host:      "localhost",
			Port:      6379,
			DB:        0,
			PoolSize:  10,
			KeyPrefix: "agentflow:",
		},
		Retry:   DefaultRetryConfig(),
		Cleanup: DefaultCleanupConfig(),
	}
}

// Serializable is an interface for objects that can be serialized to JSON
type Serializable interface {
	// MarshalJSON returns the JSON encoding of the object
	json.Marshaler
	// UnmarshalJSON parses the JSON-encoded data
	json.Unmarshaler
}

// Store is the base interface for all persistent stores
type Store interface {
	// Close closes the store and releases resources
	Close() error

	// Ping checks if the store is healthy
	Ping(ctx context.Context) error
}

package types

import "time"

// TimeoutConfig represents a common timeout configuration with default and max bounds.
type TimeoutConfig struct {
	Default time.Duration `json:"default" yaml:"default"`
	Max     time.Duration `json:"max" yaml:"max"`
}

// RetryConfig represents a common retry configuration with exponential backoff.
type RetryConfig struct {
	MaxAttempts int           `json:"max_attempts" yaml:"max_attempts"`
	Delay       time.Duration `json:"delay" yaml:"delay"`
	MaxDelay    time.Duration `json:"max_delay" yaml:"max_delay"`
}

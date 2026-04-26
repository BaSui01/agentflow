// Package common provides reusable utility functions for the AgentFlow project.
//
// This package consolidates commonly used patterns across the codebase to:
//   - Reduce code duplication
//   - Ensure consistent behavior
//   - Improve code maintainability
//
// The package includes the following modules:
//
// # Timestamp
//
// Time-related utilities for consistent timestamp handling:
//
//	now := common.TimestampNow()           // time.Now().UTC()
//	formatted := common.TimestampNowFormatted() // RFC3339 format
//
// # UUID
//
// UUID generation with convenient prefixes:
//
//	id := common.NewUUID()           // Full UUID string
//	execID := common.NewExecutionID() // "exec_" + short UUID
//	runID := common.NewRunID()       // "run_" + full UUID
//
// # HTTP
//
// HTTP response body handling utilities:
//
//	data, err := common.ReadAndClose(resp.Body)
//	err := common.DrainAndClose(resp.Body)
//
// # Errors
//
// Error wrapping and manipulation:
//
//	err := common.Wrap(baseErr, "operation failed")
//	cause := common.Cause(wrappedErr)
//
// # Nil
//
// Nil-safe operations:
//
//	if common.IsNil(value) { ... }
//	result := common.Coalesce("", "default") // Returns first non-zero
//	value := common.ValueOrDefault(ptr, defaultValue)
//
// # JSON
//
// JSON utilities with consistent error handling:
//
//	data, err := common.SafeMarshal(v)
//	cloned, err := common.JSONClone(original)
package common

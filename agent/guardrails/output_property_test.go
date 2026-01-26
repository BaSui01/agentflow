package guardrails

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 6: Output Validation Failure Logging
// Validates: Requirements 2.5 - Log all validation failure events for audit
// This property test verifies that validation failures are properly logged.
func TestProperty_OutputValidator_FailureLogging(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		auditLogger := NewMemoryAuditLogger(100)

		outputValidator := NewOutputValidator(&OutputValidatorConfig{
			EnableAuditLog: true,
			AuditLogger:    auditLogger,
		})

		// Add a failing validator
		errorCode := rapid.SampledFrom([]string{
			ErrCodePIIDetected,
			ErrCodeContentBlocked,
			ErrCodeValidationFailed,
		}).Draw(rt, "errorCode")

		errorMessage := rapid.StringMatching(`[a-zA-Z ]{10,30}`).Draw(rt, "errorMessage")

		outputValidator.AddValidator(&propMockFailingValidator{
			name:         "test_validator",
			priority:     10,
			errorCode:    errorCode,
			errorMessage: errorMessage,
			severity:     SeverityHigh,
		})

		ctx := context.Background()
		content := rapid.StringMatching(`[a-zA-Z ]{20,50}`).Draw(rt, "content")

		_, err := outputValidator.Validate(ctx, content)
		require.NoError(t, err)

		// Verify audit log entry was created
		entries := auditLogger.GetEntries()
		require.NotEmpty(t, entries, "Should have audit log entries")

		entry := entries[len(entries)-1]
		assert.Equal(t, AuditEventValidationFailed, entry.EventType, "Should be validation failed event")
		assert.Equal(t, "test_validator", entry.ValidatorName, "Should log validator name")
		assert.NotEmpty(t, entry.ContentHash, "Should have content hash")
		assert.NotEmpty(t, entry.Errors, "Should have errors in log")
		assert.False(t, entry.Timestamp.IsZero(), "Should have timestamp")
	})
}

// TestProperty_OutputValidator_AuditLogQuery tests audit log querying
func TestProperty_OutputValidator_AuditLogQuery(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		auditLogger := NewMemoryAuditLogger(100)

		// Generate multiple log entries
		numEntries := rapid.IntRange(5, 20).Draw(rt, "numEntries")
		eventTypes := []AuditEventType{
			AuditEventValidationFailed,
			AuditEventContentFiltered,
			AuditEventPIIDetected,
			AuditEventInjectionDetected,
		}

		ctx := context.Background()
		for i := 0; i < numEntries; i++ {
			entry := &AuditLogEntry{
				Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
				EventType:     rapid.SampledFrom(eventTypes).Draw(rt, fmt.Sprintf("eventType_%d", i)),
				ValidatorName: fmt.Sprintf("validator_%d", i%3),
				ContentHash:   fmt.Sprintf("hash_%d", i),
			}
			err := auditLogger.Log(ctx, entry)
			require.NoError(t, err)
		}

		// Query all entries
		allEntries, err := auditLogger.Query(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, allEntries, numEntries, "Should return all entries")

		// Query with event type filter
		filterType := rapid.SampledFrom(eventTypes).Draw(rt, "filterType")
		filtered, err := auditLogger.Query(ctx, &AuditLogFilter{
			EventTypes: []AuditEventType{filterType},
		})
		require.NoError(t, err)
		for _, entry := range filtered {
			assert.Equal(t, filterType, entry.EventType, "Filtered entries should match event type")
		}

		// Query with limit
		limit := rapid.IntRange(1, numEntries).Draw(rt, "limit")
		limited, err := auditLogger.Query(ctx, &AuditLogFilter{
			Limit: limit,
		})
		require.NoError(t, err)
		assert.LessOrEqual(t, len(limited), limit, "Should respect limit")
	})
}

// TestProperty_OutputValidator_ContentFiltering tests content filtering
func TestProperty_OutputValidator_ContentFiltering(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create content filter with blocked patterns
		blockedWord := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "blockedWord")
		filter, err := NewContentFilter(&ContentFilterConfig{
			BlockedPatterns: []string{blockedWord},
			Replacement:     "[BLOCKED]",
		})
		require.NoError(t, err)

		outputValidator := NewOutputValidator(&OutputValidatorConfig{
			Filters: []Filter{filter},
		})

		ctx := context.Background()

		// Content with blocked word
		content := "prefix " + blockedWord + " suffix"
		filtered, result, err := outputValidator.ValidateAndFilter(ctx, content)
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotContains(t, filtered, blockedWord, "Filtered content should not contain blocked word")
		assert.Contains(t, filtered, "[BLOCKED]", "Should contain replacement")
	})
}

// TestProperty_OutputValidator_SafeReplacement tests safe replacement for critical errors
func TestProperty_OutputValidator_SafeReplacement(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		safeReplacement := rapid.StringMatching(`\[SAFE: [a-z]{5,10}\]`).Draw(rt, "safeReplacement")

		outputValidator := NewOutputValidator(&OutputValidatorConfig{
			SafeReplacement: safeReplacement,
		})

		// Add validator that returns critical error
		outputValidator.AddValidator(&propMockFailingValidator{
			name:         "critical_validator",
			priority:     10,
			errorCode:    ErrCodeContentBlocked,
			errorMessage: "Critical content blocked",
			severity:     SeverityCritical,
		})

		ctx := context.Background()
		content := rapid.StringMatching(`[a-zA-Z ]{20,50}`).Draw(rt, "content")

		filtered, result, err := outputValidator.ValidateAndFilter(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Equal(t, safeReplacement, filtered, "Should return safe replacement for critical errors")

		replaced, ok := result.Metadata["replaced_with_safe_response"].(bool)
		assert.True(t, ok && replaced, "Should mark as replaced with safe response")
	})
}

// TestProperty_ContentFilter_PatternMatching tests content filter pattern matching
func TestProperty_ContentFilter_PatternMatching(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate patterns
		numPatterns := rapid.IntRange(1, 3).Draw(rt, "numPatterns")
		patterns := make([]string, numPatterns)
		for i := range patterns {
			patterns[i] = rapid.StringMatching(`[a-z]{4,8}`).Draw(rt, fmt.Sprintf("pattern_%d", i))
		}

		filter, err := NewContentFilter(&ContentFilterConfig{
			BlockedPatterns: patterns,
			Replacement:     "[X]",
			CaseSensitive:   false,
		})
		require.NoError(t, err)

		ctx := context.Background()

		// Test each pattern is detected
		for _, pattern := range patterns {
			content := "text " + pattern + " more"
			matches := filter.Detect(content)
			assert.NotEmpty(t, matches, "Should detect pattern: %s", pattern)

			filtered, err := filter.Filter(ctx, content)
			require.NoError(t, err)
			assert.NotContains(t, filtered, pattern, "Filtered should not contain pattern")
		}
	})
}

// TestProperty_ContentFilter_AddRemovePattern tests dynamic pattern management
func TestProperty_ContentFilter_AddRemovePattern(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		filter, err := NewContentFilter(&ContentFilterConfig{
			Replacement: "[REMOVED]",
		})
		require.NoError(t, err)

		// Add patterns
		numPatterns := rapid.IntRange(2, 5).Draw(rt, "numPatterns")
		patterns := make([]string, numPatterns)
		for i := range patterns {
			patterns[i] = rapid.StringMatching(`[a-z]{5,8}`).Draw(rt, fmt.Sprintf("pattern_%d", i))
			err := filter.AddPattern(patterns[i])
			require.NoError(t, err)
		}

		assert.Len(t, filter.GetPatterns(), numPatterns, "Should have all patterns")

		// Remove one pattern
		removeIndex := rapid.IntRange(0, numPatterns-1).Draw(rt, "removeIndex")
		removed := filter.RemovePattern(patterns[removeIndex])
		assert.True(t, removed, "Should successfully remove pattern")
		assert.Len(t, filter.GetPatterns(), numPatterns-1, "Should have one less pattern")
	})
}

// TestProperty_AuditLogger_MaxSize tests audit logger max size enforcement
func TestProperty_AuditLogger_MaxSize(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		maxSize := rapid.IntRange(5, 20).Draw(rt, "maxSize")
		logger := NewMemoryAuditLogger(maxSize)

		ctx := context.Background()

		// Add more entries than max size
		numEntries := maxSize + rapid.IntRange(5, 15).Draw(rt, "extraEntries")
		for i := 0; i < numEntries; i++ {
			entry := &AuditLogEntry{
				Timestamp:     time.Now(),
				EventType:     AuditEventValidationFailed,
				ValidatorName: fmt.Sprintf("validator_%d", i),
				ContentHash:   fmt.Sprintf("hash_%d", i),
			}
			err := logger.Log(ctx, entry)
			require.NoError(t, err)
		}

		// Should not exceed max size
		entries := logger.GetEntries()
		assert.LessOrEqual(t, len(entries), maxSize, "Should not exceed max size")
	})
}

// TestProperty_AuditLogger_TimeRangeFilter tests time range filtering
func TestProperty_AuditLogger_TimeRangeFilter(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		logger := NewMemoryAuditLogger(100)
		ctx := context.Background()

		baseTime := time.Now()

		// Add entries with different timestamps
		for i := 0; i < 10; i++ {
			entry := &AuditLogEntry{
				Timestamp:     baseTime.Add(time.Duration(i) * time.Hour),
				EventType:     AuditEventValidationFailed,
				ValidatorName: fmt.Sprintf("validator_%d", i),
				ContentHash:   fmt.Sprintf("hash_%d", i),
			}
			err := logger.Log(ctx, entry)
			require.NoError(t, err)
		}

		// Query with time range
		startTime := baseTime.Add(2 * time.Hour)
		endTime := baseTime.Add(7 * time.Hour)

		filtered, err := logger.Query(ctx, &AuditLogFilter{
			StartTime: &startTime,
			EndTime:   &endTime,
		})
		require.NoError(t, err)

		for _, entry := range filtered {
			assert.True(t, !entry.Timestamp.Before(startTime), "Entry should be after start time")
			assert.True(t, !entry.Timestamp.After(endTime), "Entry should be before end time")
		}
	})
}

// TestProperty_ContentFilterValidator_Integration tests ContentFilterValidator integration
func TestProperty_ContentFilterValidator_Integration(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		blockedWord := rapid.StringMatching(`[a-z]{5,10}`).Draw(rt, "blockedWord")

		filter, err := NewContentFilter(&ContentFilterConfig{
			BlockedPatterns: []string{blockedWord},
			Replacement:     "[FILTERED]",
		})
		require.NoError(t, err)

		validator := NewContentFilterValidator(filter, 50)

		ctx := context.Background()

		// Content with blocked word
		content := "some " + blockedWord + " text"
		result, err := validator.Validate(ctx, content)
		require.NoError(t, err)
		assert.False(t, result.Valid, "Should detect blocked content")
		assert.Equal(t, ErrCodeContentBlocked, result.Errors[0].Code)

		// Check filtered content in metadata
		filtered, ok := result.Metadata["filtered_content"].(string)
		require.True(t, ok, "Should have filtered_content")
		assert.NotContains(t, filtered, blockedWord)
	})
}

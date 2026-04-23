package core

import (
	"fmt"
	"strings"
	"time"
)

// ObservabilityRunner mirrors the root-level optional observability contract.
type ObservabilityRunner interface {
	StartTrace(traceID, agentID string)
	EndTrace(traceID, status string, err error)
	RecordTask(agentID string, success bool, duration time.Duration, tokens int, cost, quality float64)
}

// ExplainabilityTimelineRecorder mirrors the optional explainability extension.
type ExplainabilityTimelineRecorder interface {
	AddExplainabilityTimeline(traceID, entryType, summary string, metadata map[string]any)
}

// NormalizeInstructionList removes empty items and preserves first-seen order.
func NormalizeInstructionList(instructions []string) []string {
	if len(instructions) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(instructions))
	cleaned := make([]string, 0, len(instructions))
	for _, instruction := range instructions {
		instruction = strings.TrimSpace(instruction)
		if instruction == "" {
			continue
		}
		if _, exists := unique[instruction]; exists {
			continue
		}
		unique[instruction] = struct{}{}
		cleaned = append(cleaned, instruction)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

// ExplainabilityTimelineRecorderFrom narrows observability to the optional recorder.
func ExplainabilityTimelineRecorderFrom(obs ObservabilityRunner) ExplainabilityTimelineRecorder {
	recorder, _ := obs.(ExplainabilityTimelineRecorder)
	return recorder
}

// AppendUniqueString appends a trimmed string if it is not already present.
func AppendUniqueString(values []string, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(strings.TrimSpace(existing), trimmed) {
			return values
		}
	}
	return append(values, trimmed)
}

// FallbackString returns the first non-empty trimmed value.
func FallbackString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// PanicPayloadToError normalizes recover() payloads into errors.
func PanicPayloadToError(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", v)
}

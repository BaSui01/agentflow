package checkpointcore

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

type Observation struct {
	Stage     string
	Content   string
	Error     string
	Metadata  map[string]any
	Iteration int
}

type ValidationResult struct {
	AcceptanceCriteria []string
	UnresolvedItems    []string
	RemainingRisks     []string
	Status             string
	Summary            string
	Reason             string
}

func Logger(logger *zap.Logger, store string) *zap.Logger {
	if logger == nil {
		logger = zap.NewNop()
	}
	return logger.With(zap.String("store", store))
}

func NextCheckpointID(counter *uint64) string {
	next := atomic.AddUint64(counter, 1)
	return fmt.Sprintf("ckpt_%d%d", time.Now().UnixNano(), next)
}

func CloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func NormalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func BuildLoopPlanID(loopStateID string, planVersion int) string {
	base := strings.TrimSpace(loopStateID)
	if base == "" {
		base = "loop"
	}
	return fmt.Sprintf("%s-plan-%d", base, planVersion)
}

func DerivePlanVersion(observations []Observation) int {
	count := 0
	for _, observation := range observations {
		if observation.Stage == "plan" {
			count++
		}
	}
	return count
}

func SummarizeObservations(observations []Observation) string {
	if len(observations) == 0 {
		return ""
	}
	parts := make([]string, 0, 3)
	start := len(observations) - 3
	if start < 0 {
		start = 0
	}
	for _, observation := range observations[start:] {
		part := observation.Stage
		if text := strings.TrimSpace(observation.Error); text != "" {
			part += ":" + SummarizeText(text)
		} else if text := strings.TrimSpace(observation.Content); text != "" {
			part += ":" + SummarizeText(text)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " | ")
}

func SummarizeLastOutput(lastOutput string, observations []Observation) string {
	if text := strings.TrimSpace(lastOutput); text != "" {
		return SummarizeText(text)
	}
	for i := len(observations) - 1; i >= 0; i-- {
		observation := observations[i]
		if observation.Stage == "act" {
			if text := strings.TrimSpace(observation.Content); text != "" {
				return SummarizeText(text)
			}
		}
	}
	return ""
}

func SummarizeLastError(observations []Observation) string {
	for i := len(observations) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(observations[i].Error); text != "" {
			return SummarizeText(text)
		}
	}
	return ""
}

func SummarizeValidationState(status string, unresolvedItems, remainingRisks []string) string {
	if len(unresolvedItems) == 0 && len(remainingRisks) == 0 {
		switch status {
		case "passed":
			return "validation passed"
		case "pending":
			return "validation pending"
		case "failed":
			return "validation failed"
		default:
			return ""
		}
	}
	parts := make([]string, 0, 2)
	if len(unresolvedItems) > 0 {
		parts = append(parts, "unresolved: "+strings.Join(unresolvedItems, ", "))
	}
	if len(remainingRisks) > 0 {
		parts = append(parts, "risks: "+strings.Join(remainingRisks, ", "))
	}
	return strings.Join(parts, "; ")
}

func SummarizeText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= 160 {
		return trimmed
	}
	return string(runes[:160]) + "..."
}

func ContextString(values map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if ok && value != "" {
			return value, true
		}
	}
	return "", false
}

func ContextStrings(values map[string]any, keys ...string) ([]string, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return []string{trimmed}, true
			}
		case []string:
			return append([]string(nil), typed...), true
		case []any:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				text, ok := item.(string)
				if ok && text != "" {
					result = append(result, text)
				}
			}
			if len(result) > 0 {
				return result, true
			}
		}
	}
	return nil, false
}

func ContextInt(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case int:
			return typed, true
		case int32:
			return int(typed), true
		case int64:
			return int(typed), true
		case float64:
			return int(typed), true
		}
	}
	return 0, false
}

func ContextFloat(values map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		}
	}
	return 0, false
}

func ContextBool(values map[string]any, keys ...string) (value, ok bool) {
	for _, key := range keys {
		raw, found := values[key]
		if !found {
			continue
		}
		value, ok = raw.(bool)
		if ok {
			return value, true
		}
	}
	return false, false
}

func FirstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func ContextStringValue(values map[string]any, keys ...string) string {
	if value, ok := ContextString(values, keys...); ok {
		return value
	}
	return ""
}

func CompareMessageCounts(count1, count2 int) string {
	if count1 == count2 {
		return fmt.Sprintf("No change (%d messages)", count1)
	}
	return fmt.Sprintf("Changed from %d to %d messages", count1, count2)
}

func CompareMetadata(meta1, meta2 map[string]any) string {
	added := 0
	removed := 0
	changed := 0

	for key, value2 := range meta2 {
		if value1, exists := meta1[key]; !exists {
			added++
		} else if fmt.Sprintf("%v", value1) != fmt.Sprintf("%v", value2) {
			changed++
		}
	}

	for key := range meta1 {
		if _, exists := meta2[key]; !exists {
			removed++
		}
	}

	if added == 0 && removed == 0 && changed == 0 {
		return "No changes"
	}

	return fmt.Sprintf("Added: %d, Removed: %d, Changed: %d", added, removed, changed)
}

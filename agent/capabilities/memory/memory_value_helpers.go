package memory

import (
	"time"
)

func extractMemoryAgentID(memory any) string {
	m, ok := memory.(map[string]any)
	if !ok {
		return ""
	}
	agentID, _ := m["agent_id"].(string)
	return agentID
}

func extractMemoryTimestamp(memory any) time.Time {
	m, ok := memory.(map[string]any)
	if !ok {
		return time.Time{}
	}

	switch v := m["timestamp"].(type) {
	case time.Time:
		return v
	case *time.Time:
		if v != nil {
			return *v
		}
	case int64:
		return time.Unix(0, v)
	case float64:
		return time.Unix(0, int64(v))
	case string:
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

func extractMemoryContent(memory any) string {
	m, ok := memory.(map[string]any)
	if !ok {
		return ""
	}
	content, _ := m["content"].(string)
	return content
}

func extractMemoryMetadata(memory any) map[string]any {
	m, ok := memory.(map[string]any)
	if !ok {
		return nil
	}

	raw := m["metadata"]
	if raw == nil {
		return nil
	}

	switch v := raw.(type) {
	case map[string]any:
		return v
	default:
		return nil
	}
}

func extractMemoryVector(memory any) ([]float64, bool) {
	m, ok := memory.(map[string]any)
	if !ok {
		return nil, false
	}

	for _, raw := range []any{m["vector"], m["embedding"]} {
		if vec, ok := coerceVector(raw); ok {
			return vec, true
		}
	}

	meta := extractMemoryMetadata(memory)
	if meta != nil {
		for _, raw := range []any{meta["vector"], meta["embedding"]} {
			if vec, ok := coerceVector(raw); ok {
				return vec, true
			}
		}
	}

	return nil, false
}

func coerceVector(raw any) ([]float64, bool) {
	switch v := raw.(type) {
	case []float64:
		return append([]float64(nil), v...), true
	case []float32:
		out := make([]float64, 0, len(v))
		for _, x := range v {
			out = append(out, float64(x))
		}
		return out, true
	case []any:
		out := make([]float64, 0, len(v))
		for _, x := range v {
			switch n := x.(type) {
			case float64:
				out = append(out, n)
			case float32:
				out = append(out, float64(n))
			case int:
				out = append(out, float64(n))
			case int64:
				out = append(out, float64(n))
			default:
				return nil, false
			}
		}
		return out, true
	default:
		return nil, false
	}
}

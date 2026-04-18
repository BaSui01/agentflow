package agent

import "strings"

func contextBool(input *Input, key string) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func contextString(input *Input, key string) string {
	if input == nil || len(input.Context) == 0 {
		return ""
	}
	value, ok := input.Context[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func intContextAtLeast(input *Input, key string, min int) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case int:
		return typed >= min
	case int32:
		return int(typed) >= min
	case int64:
		return int(typed) >= min
	case float64:
		return int(typed) >= min
	default:
		return false
	}
}

func contentContainsAny(input *Input, terms ...string) bool {
	if input == nil {
		return false
	}
	content := strings.ToLower(input.Content)
	for _, term := range terms {
		if strings.Contains(content, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

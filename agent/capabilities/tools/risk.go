package tools

import "strings"

const (
	ToolRiskSafeRead         = "safe_read"
	ToolRiskRequiresApproval = "requires_approval"
	ToolRiskUnknown          = "unknown"
)

func ClassifyToolRiskByName(name string) string {
	switch strings.TrimSpace(name) {
	case "web_search", "file_search", "retrieval", "read_file", "list_directory":
		return ToolRiskSafeRead
	case "write_file", "edit_file", "run_command", "code_execution":
		return ToolRiskRequiresApproval
	default:
		if strings.HasPrefix(strings.TrimSpace(name), "mcp_") {
			return ToolRiskRequiresApproval
		}
		return ToolRiskUnknown
	}
}

func GroupToolRisks(names []string) map[string][]string {
	grouped := map[string][]string{
		ToolRiskSafeRead:         {},
		ToolRiskRequiresApproval: {},
		ToolRiskUnknown:          {},
	}
	for _, name := range normalizeNames(names) {
		risk := ClassifyToolRiskByName(name)
		grouped[risk] = append(grouped[risk], name)
	}
	return grouped
}

func normalizeNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

package agent

import "strings"

const (
	toolRiskSafeRead         = "safe_read"
	toolRiskRequiresApproval = "requires_approval"
	toolRiskUnknown          = "unknown"
)

func classifyToolRiskByName(name string) string {
	switch strings.TrimSpace(name) {
	case "web_search", "file_search", "retrieval", "read_file", "list_directory":
		return toolRiskSafeRead
	case "write_file", "edit_file", "run_command", "code_execution":
		return toolRiskRequiresApproval
	default:
		if strings.HasPrefix(strings.TrimSpace(name), "mcp_") {
			return toolRiskRequiresApproval
		}
		return toolRiskUnknown
	}
}

func groupToolRisks(names []string) map[string][]string {
	grouped := map[string][]string{
		toolRiskSafeRead:         {},
		toolRiskRequiresApproval: {},
		toolRiskUnknown:          {},
	}
	for _, name := range normalizeStringSlice(names) {
		risk := classifyToolRiskByName(name)
		grouped[risk] = append(grouped[risk], name)
	}
	return grouped
}

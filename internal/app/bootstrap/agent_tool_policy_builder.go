package bootstrap

import (
	"strings"
	"time"

	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

const (
	defaultHostedToolApprovalRuleID = "hosted-tools-default-approval"
	defaultHostedToolAllowRuleID    = "hosted-tools-default-allow"
)

func newDefaultToolPermissionManager(logger *zap.Logger) llmtools.PermissionManager {
	manager := llmtools.NewPermissionManager(logger)
	now := time.Now()
	_ = manager.AddRule(&llmtools.PermissionRule{
		ID:          defaultHostedToolApprovalRuleID,
		Name:        "Default hosted tool approval",
		Description: "Mutating or execution-capable hosted tools require explicit approval by default.",
		ToolPattern: "*",
		Decision:    llmtools.PermissionRequireApproval,
		Priority:    200,
		Conditions: []llmtools.RuleCondition{
			{
				Field:    "hosted_tool_risk",
				Operator: "eq",
				Value:    "requires_approval",
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = manager.AddRule(&llmtools.PermissionRule{
		ID:          defaultHostedToolAllowRuleID,
		Name:        "Default hosted tool allow",
		Description: "Read-only hosted tools are allowed by default.",
		ToolPattern: "*",
		Decision:    llmtools.PermissionAllow,
		Priority:    100,
		Conditions: []llmtools.RuleCondition{
			{
				Field:    "hosted_tool_risk",
				Operator: "eq",
				Value:    "safe_read",
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	return manager
}

func filterToolSchemasByAgentPermission(
	pm llmtools.PermissionManager,
	agentID string,
	schemas []types.ToolSchema,
) []types.ToolSchema {
	if pm == nil || strings.TrimSpace(agentID) == "" || len(schemas) == 0 {
		return schemas
	}

	perm, ok := pm.GetAgentPermission(agentID)
	if !ok || perm == nil {
		return schemas
	}

	filtered := make([]types.ToolSchema, 0, len(schemas))
	for _, schema := range schemas {
		if !toolSchemaAllowedForAgent(perm, schema.Name) {
			continue
		}
		filtered = append(filtered, schema)
	}
	return filtered
}

func toolSchemaAllowedForAgent(perm *llmtools.AgentPermission, toolName string) bool {
	name := strings.TrimSpace(toolName)
	if name == "" || perm == nil {
		return false
	}

	for _, denied := range perm.DeniedTools {
		if permissionPatternMatches(denied, name) {
			return false
		}
	}

	if len(perm.AllowedTools) == 0 {
		return true
	}

	for _, allowed := range perm.AllowedTools {
		if permissionPatternMatches(allowed, name) {
			return true
		}
	}
	return false
}

func permissionPatternMatches(pattern string, value string) bool {
	p := strings.TrimSpace(pattern)
	v := strings.TrimSpace(value)
	if p == "" || v == "" {
		return false
	}
	if p == "*" {
		return true
	}
	if strings.HasSuffix(p, "*") {
		return strings.HasPrefix(v, strings.TrimSuffix(p, "*"))
	}
	if strings.HasPrefix(p, "*") {
		return strings.HasSuffix(v, strings.TrimPrefix(p, "*"))
	}
	return p == v
}

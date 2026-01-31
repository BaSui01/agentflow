// Package tools provides tool permission control for enterprise AI Agent frameworks.
package tools

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// PermissionDecision represents the result of a permission check.
type PermissionDecision string

const (
	PermissionAllow           PermissionDecision = "allow"
	PermissionDeny            PermissionDecision = "deny"
	PermissionRequireApproval PermissionDecision = "require_approval"
)

// PermissionRule defines a permission rule for tool access.
type PermissionRule struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	ToolPattern string             `json:"tool_pattern"`          // Tool name pattern (supports wildcards)
	Decision    PermissionDecision `json:"decision"`              // allow/deny/require_approval
	Priority    int                `json:"priority"`              // Higher priority rules are evaluated first
	Conditions  []RuleCondition    `json:"conditions,omitempty"`  // Additional conditions
	ValidFrom   *time.Time         `json:"valid_from,omitempty"`  // Permission start time
	ValidUntil  *time.Time         `json:"valid_until,omitempty"` // Permission expiry time
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// RuleCondition defines additional conditions for permission rules.
type RuleCondition struct {
	Type     string `json:"type"`     // time_range, ip_range, parameter_check, etc.
	Operator string `json:"operator"` // eq, ne, gt, lt, contains, matches
	Field    string `json:"field"`    // Field to check
	Value    string `json:"value"`    // Expected value
}

// PermissionContext provides context for permission checks.
type PermissionContext struct {
	AgentID    string            `json:"agent_id"`
	UserID     string            `json:"user_id"`
	Roles      []string          `json:"roles"`
	ToolName   string            `json:"tool_name"`
	Arguments  map[string]any    `json:"arguments,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	RequestIP  string            `json:"request_ip,omitempty"`
	RequestAt  time.Time         `json:"request_at"`
	TraceID    string            `json:"trace_id,omitempty"`
	SessionID  string            `json:"session_id,omitempty"`
}

// PermissionCheckResult contains the result of a permission check.
type PermissionCheckResult struct {
	Decision     PermissionDecision `json:"decision"`
	MatchedRule  *PermissionRule    `json:"matched_rule,omitempty"`
	Reason       string             `json:"reason"`
	ApprovalID   string             `json:"approval_id,omitempty"` // For require_approval decisions
	CheckedAt    time.Time          `json:"checked_at"`
	CheckLatency time.Duration      `json:"check_latency"`
}

// Role defines a role with associated permissions.
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	ParentRoles []string `json:"parent_roles,omitempty"` // For role inheritance
	Permissions []string `json:"permissions,omitempty"`  // Permission rule IDs
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentPermission defines agent-specific permissions.
type AgentPermission struct {
	AgentID       string   `json:"agent_id"`
	AllowedTools  []string `json:"allowed_tools,omitempty"`  // Explicit allow list
	DeniedTools   []string `json:"denied_tools,omitempty"`   // Explicit deny list
	InheritFrom   string   `json:"inherit_from,omitempty"`   // Parent agent ID
	MaxCallsPerHour int    `json:"max_calls_per_hour,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PermissionManager manages tool permissions.
type PermissionManager interface {
	// CheckPermission checks if a tool call is permitted.
	CheckPermission(ctx context.Context, permCtx *PermissionContext) (*PermissionCheckResult, error)

	// AddRule adds a permission rule.
	AddRule(rule *PermissionRule) error

	// RemoveRule removes a permission rule.
	RemoveRule(ruleID string) error

	// GetRule retrieves a permission rule by ID.
	GetRule(ruleID string) (*PermissionRule, bool)

	// ListRules lists all permission rules.
	ListRules() []*PermissionRule

	// AddRole adds a role.
	AddRole(role *Role) error

	// RemoveRole removes a role.
	RemoveRole(roleID string) error

	// GetRole retrieves a role by ID.
	GetRole(roleID string) (*Role, bool)

	// AssignRoleToUser assigns a role to a user.
	AssignRoleToUser(userID, roleID string) error

	// GetUserRoles gets all roles for a user.
	GetUserRoles(userID string) []string

	// SetAgentPermission sets agent-specific permissions.
	SetAgentPermission(perm *AgentPermission) error

	// GetAgentPermission gets agent-specific permissions.
	GetAgentPermission(agentID string) (*AgentPermission, bool)
}

// DefaultPermissionManager is the default implementation of PermissionManager.
type DefaultPermissionManager struct {
	rules            map[string]*PermissionRule
	roles            map[string]*Role
	userRoles        map[string][]string // userID -> roleIDs
	agentPermissions map[string]*AgentPermission
	approvalHandler  ApprovalHandler
	logger           *zap.Logger
	mu               sync.RWMutex
}

// ApprovalHandler handles approval requests for require_approval decisions.
type ApprovalHandler interface {
	RequestApproval(ctx context.Context, permCtx *PermissionContext, rule *PermissionRule) (approvalID string, err error)
	CheckApprovalStatus(ctx context.Context, approvalID string) (approved bool, err error)
}

// NewPermissionManager creates a new permission manager.
func NewPermissionManager(logger *zap.Logger) *DefaultPermissionManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DefaultPermissionManager{
		rules:            make(map[string]*PermissionRule),
		roles:            make(map[string]*Role),
		userRoles:        make(map[string][]string),
		agentPermissions: make(map[string]*AgentPermission),
		logger:           logger.With(zap.String("component", "permission_manager")),
	}
}

// SetApprovalHandler sets the approval handler.
func (pm *DefaultPermissionManager) SetApprovalHandler(handler ApprovalHandler) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.approvalHandler = handler
}

// CheckPermission checks if a tool call is permitted.
func (pm *DefaultPermissionManager) CheckPermission(ctx context.Context, permCtx *PermissionContext) (*PermissionCheckResult, error) {
	start := time.Now()
	result := &PermissionCheckResult{
		CheckedAt: start,
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 1. Check agent-specific permissions first
	if agentPerm, ok := pm.agentPermissions[permCtx.AgentID]; ok {
		decision := pm.checkAgentPermission(agentPerm, permCtx.ToolName)
		if decision != "" {
			result.Decision = decision
			result.Reason = fmt.Sprintf("agent-specific permission: %s", decision)
			result.CheckLatency = time.Since(start)
			return result, nil
		}
	}

	// 2. Collect all applicable roles (including inherited roles)
	allRoles := pm.collectRoles(permCtx.UserID, permCtx.Roles)

	// 3. Collect all applicable rules
	applicableRules := pm.collectApplicableRules(permCtx.ToolName, allRoles)

	// 4. Sort rules by priority (higher first)
	sortRulesByPriority(applicableRules)

	// 5. Evaluate rules
	for _, rule := range applicableRules {
		if pm.evaluateRule(rule, permCtx) {
			result.Decision = rule.Decision
			result.MatchedRule = rule
			result.Reason = fmt.Sprintf("matched rule: %s", rule.Name)

			// Handle require_approval
			if rule.Decision == PermissionRequireApproval && pm.approvalHandler != nil {
				approvalID, err := pm.approvalHandler.RequestApproval(ctx, permCtx, rule)
				if err != nil {
					pm.logger.Error("failed to request approval", zap.Error(err))
					result.Decision = PermissionDeny
					result.Reason = "approval request failed"
				} else {
					result.ApprovalID = approvalID
				}
			}

			result.CheckLatency = time.Since(start)
			return result, nil
		}
	}

	// 6. Default: deny if no rule matches
	result.Decision = PermissionDeny
	result.Reason = "no matching permission rule"
	result.CheckLatency = time.Since(start)

	pm.logger.Debug("permission check completed",
		zap.String("agent_id", permCtx.AgentID),
		zap.String("user_id", permCtx.UserID),
		zap.String("tool", permCtx.ToolName),
		zap.String("decision", string(result.Decision)),
		zap.Duration("latency", result.CheckLatency),
	)

	return result, nil
}

// checkAgentPermission checks agent-specific permissions.
func (pm *DefaultPermissionManager) checkAgentPermission(perm *AgentPermission, toolName string) PermissionDecision {
	// Check explicit deny list first
	for _, denied := range perm.DeniedTools {
		if matchPattern(denied, toolName) {
			return PermissionDeny
		}
	}

	// Check explicit allow list
	for _, allowed := range perm.AllowedTools {
		if matchPattern(allowed, toolName) {
			return PermissionAllow
		}
	}

	// Check inherited permissions
	if perm.InheritFrom != "" {
		if parentPerm, ok := pm.agentPermissions[perm.InheritFrom]; ok {
			return pm.checkAgentPermission(parentPerm, toolName)
		}
	}

	return "" // No decision, continue to other checks
}

// collectRoles collects all applicable roles including inherited ones.
func (pm *DefaultPermissionManager) collectRoles(userID string, explicitRoles []string) []string {
	roleSet := make(map[string]bool)

	// Add explicit roles
	for _, r := range explicitRoles {
		roleSet[r] = true
	}

	// Add user's assigned roles
	if userRoles, ok := pm.userRoles[userID]; ok {
		for _, r := range userRoles {
			roleSet[r] = true
		}
	}

	// Expand inherited roles
	var expandRoles func(roleID string)
	expandRoles = func(roleID string) {
		if role, ok := pm.roles[roleID]; ok {
			for _, parentID := range role.ParentRoles {
				if !roleSet[parentID] {
					roleSet[parentID] = true
					expandRoles(parentID)
				}
			}
		}
	}

	for roleID := range roleSet {
		expandRoles(roleID)
	}

	result := make([]string, 0, len(roleSet))
	for r := range roleSet {
		result = append(result, r)
	}
	return result
}

// collectApplicableRules collects all rules applicable to the tool and roles.
func (pm *DefaultPermissionManager) collectApplicableRules(toolName string, roles []string) []*PermissionRule {
	var rules []*PermissionRule

	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r] = true
	}

	for _, rule := range pm.rules {
		// Check if tool pattern matches
		if !matchPattern(rule.ToolPattern, toolName) {
			continue
		}

		// Check time validity
		now := time.Now()
		if rule.ValidFrom != nil && now.Before(*rule.ValidFrom) {
			continue
		}
		if rule.ValidUntil != nil && now.After(*rule.ValidUntil) {
			continue
		}

		rules = append(rules, rule)
	}

	return rules
}

// evaluateRule evaluates a rule against the permission context.
func (pm *DefaultPermissionManager) evaluateRule(rule *PermissionRule, permCtx *PermissionContext) bool {
	// Evaluate all conditions
	for _, cond := range rule.Conditions {
		if !pm.evaluateCondition(cond, permCtx) {
			return false
		}
	}
	return true
}

// evaluateCondition evaluates a single condition.
func (pm *DefaultPermissionManager) evaluateCondition(cond RuleCondition, permCtx *PermissionContext) bool {
	var fieldValue string

	switch cond.Field {
	case "agent_id":
		fieldValue = permCtx.AgentID
	case "user_id":
		fieldValue = permCtx.UserID
	case "request_ip":
		fieldValue = permCtx.RequestIP
	case "hour":
		fieldValue = fmt.Sprintf("%d", permCtx.RequestAt.Hour())
	default:
		if permCtx.Metadata != nil {
			fieldValue = permCtx.Metadata[cond.Field]
		}
	}

	switch cond.Operator {
	case "eq":
		return fieldValue == cond.Value
	case "ne":
		return fieldValue != cond.Value
	case "contains":
		return containsString(fieldValue, cond.Value)
	case "matches":
		return matchPattern(cond.Value, fieldValue)
	default:
		return false
	}
}

// AddRule adds a permission rule.
func (pm *DefaultPermissionManager) AddRule(rule *PermissionRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	pm.rules[rule.ID] = rule

	pm.logger.Info("permission rule added",
		zap.String("rule_id", rule.ID),
		zap.String("name", rule.Name),
		zap.String("decision", string(rule.Decision)),
	)

	return nil
}

// RemoveRule removes a permission rule.
func (pm *DefaultPermissionManager) RemoveRule(ruleID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.rules[ruleID]; !ok {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	delete(pm.rules, ruleID)
	pm.logger.Info("permission rule removed", zap.String("rule_id", ruleID))
	return nil
}

// GetRule retrieves a permission rule by ID.
func (pm *DefaultPermissionManager) GetRule(ruleID string) (*PermissionRule, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	rule, ok := pm.rules[ruleID]
	return rule, ok
}

// ListRules lists all permission rules.
func (pm *DefaultPermissionManager) ListRules() []*PermissionRule {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rules := make([]*PermissionRule, 0, len(pm.rules))
	for _, rule := range pm.rules {
		rules = append(rules, rule)
	}
	return rules
}

// AddRole adds a role.
func (pm *DefaultPermissionManager) AddRole(role *Role) error {
	if role.ID == "" {
		return fmt.Errorf("role ID is required")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()
	pm.roles[role.ID] = role

	pm.logger.Info("role added",
		zap.String("role_id", role.ID),
		zap.String("name", role.Name),
	)

	return nil
}

// RemoveRole removes a role.
func (pm *DefaultPermissionManager) RemoveRole(roleID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.roles[roleID]; !ok {
		return fmt.Errorf("role not found: %s", roleID)
	}

	delete(pm.roles, roleID)
	pm.logger.Info("role removed", zap.String("role_id", roleID))
	return nil
}

// GetRole retrieves a role by ID.
func (pm *DefaultPermissionManager) GetRole(roleID string) (*Role, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	role, ok := pm.roles[roleID]
	return role, ok
}

// AssignRoleToUser assigns a role to a user.
func (pm *DefaultPermissionManager) AssignRoleToUser(userID, roleID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.roles[roleID]; !ok {
		return fmt.Errorf("role not found: %s", roleID)
	}

	pm.userRoles[userID] = append(pm.userRoles[userID], roleID)
	pm.logger.Info("role assigned to user",
		zap.String("user_id", userID),
		zap.String("role_id", roleID),
	)
	return nil
}

// GetUserRoles gets all roles for a user.
func (pm *DefaultPermissionManager) GetUserRoles(userID string) []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return append([]string{}, pm.userRoles[userID]...)
}

// SetAgentPermission sets agent-specific permissions.
func (pm *DefaultPermissionManager) SetAgentPermission(perm *AgentPermission) error {
	if perm.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	perm.CreatedAt = time.Now()
	perm.UpdatedAt = time.Now()
	pm.agentPermissions[perm.AgentID] = perm

	pm.logger.Info("agent permission set",
		zap.String("agent_id", perm.AgentID),
		zap.Int("allowed_tools", len(perm.AllowedTools)),
		zap.Int("denied_tools", len(perm.DeniedTools)),
	)

	return nil
}

// GetAgentPermission gets agent-specific permissions.
func (pm *DefaultPermissionManager) GetAgentPermission(agentID string) (*AgentPermission, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	perm, ok := pm.agentPermissions[agentID]
	return perm, ok
}

// Helper functions

func sortRulesByPriority(rules []*PermissionRule) {
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
}

func matchPattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == value {
		return true
	}
	// Simple wildcard matching
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(value) >= len(prefix) && value[:len(prefix)] == prefix
	}
	if len(pattern) > 0 && pattern[0] == '*' {
		suffix := pattern[1:]
		return len(value) >= len(suffix) && value[len(value)-len(suffix):] == suffix
	}
	return false
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstr(s, substr)
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// PermissionMiddleware creates a middleware that checks permissions before tool execution.
func PermissionMiddleware(pm PermissionManager) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args []byte) ([]byte, error) {
			// Extract permission context from context
			permCtx, ok := ctx.Value(permissionContextKey).(*PermissionContext)
			if !ok {
				return nil, fmt.Errorf("permission context not found")
			}

			// Check permission
			result, err := pm.CheckPermission(ctx, permCtx)
			if err != nil {
				return nil, fmt.Errorf("permission check failed: %w", err)
			}

			switch result.Decision {
			case PermissionAllow:
				return next(ctx, args)
			case PermissionDeny:
				return nil, fmt.Errorf("permission denied: %s", result.Reason)
			case PermissionRequireApproval:
				return nil, fmt.Errorf("approval required (ID: %s): %s", result.ApprovalID, result.Reason)
			default:
				return nil, fmt.Errorf("unknown permission decision: %s", result.Decision)
			}
		}
	}
}

type contextKey string

const permissionContextKey contextKey = "permission_context"

// WithPermissionContext adds permission context to the context.
func WithPermissionContext(ctx context.Context, permCtx *PermissionContext) context.Context {
	return context.WithValue(ctx, permissionContextKey, permCtx)
}

// GetPermissionContext retrieves permission context from the context.
func GetPermissionContext(ctx context.Context) (*PermissionContext, bool) {
	permCtx, ok := ctx.Value(permissionContextKey).(*PermissionContext)
	return permCtx, ok
}

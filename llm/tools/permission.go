// 软件包工具为企业AI代理框架提供工具许可控制.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 权限决定代表权限检查的结果.
type PermissionDecision string

const (
	PermissionAllow           PermissionDecision = "allow"
	PermissionDeny            PermissionDecision = "deny"
	PermissionRequireApproval PermissionDecision = "require_approval"
)

// 权限规则定义了工具访问的权限规则.
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

// 规则条件定义了许可规则的额外条件.
type RuleCondition struct {
	Type     string `json:"type"`     // time_range, ip_range, parameter_check, etc.
	Operator string `json:"operator"` // eq, ne, gt, lt, contains, matches
	Field    string `json:"field"`    // Field to check
	Value    string `json:"value"`    // Expected value
}

// 权限Context为权限检查提供了上下文.
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

// 权限检查结果 。
type PermissionCheckResult struct {
	Decision     PermissionDecision `json:"decision"`
	MatchedRule  *PermissionRule    `json:"matched_rule,omitempty"`
	Reason       string             `json:"reason"`
	ApprovalID   string             `json:"approval_id,omitempty"` // For require_approval decisions
	CheckedAt    time.Time          `json:"checked_at"`
	CheckLatency time.Duration      `json:"check_latency"`
}

// 角色定义一个带有相关权限的角色.
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	ParentRoles []string `json:"parent_roles,omitempty"` // For role inheritance
	Permissions []string `json:"permissions,omitempty"`  // Permission rule IDs
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentPermission定义了特定代理权限.
type AgentPermission struct {
	AgentID       string   `json:"agent_id"`
	AllowedTools  []string `json:"allowed_tools,omitempty"`  // Explicit allow list
	DeniedTools   []string `json:"denied_tools,omitempty"`   // Explicit deny list
	InheritFrom   string   `json:"inherit_from,omitempty"`   // Parent agent ID
	MaxCallsPerHour int    `json:"max_calls_per_hour,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// 权限管理器管理工具权限 。
type PermissionManager interface {
	// 检查是否允许调用工具 。
	CheckPermission(ctx context.Context, permCtx *PermissionContext) (*PermissionCheckResult, error)

	// 添加规则添加了权限规则 。
	AddRule(rule *PermissionRule) error

	// 删除规则删除权限规则 。
	RemoveRule(ruleID string) error

	// Get Rule 以 ID 检索许可规则 。
	GetRule(ruleID string) (*PermissionRule, bool)

	// List Rules 列出所有许可规则 。
	ListRules() []*PermissionRule

	// 添加Role增加了一个角色.
	AddRole(role *Role) error

	// 删除 Role 删除一个角色 。
	RemoveRole(roleID string) error

	// GetRole通过ID检索角色.
	GetRole(roleID string) (*Role, bool)

	// AssignRoleToUser为用户分配一个角色.
	AssignRoleToUser(userID, roleID string) error

	// GetUserRoles为用户获得所有角色.
	GetUserRoles(userID string) []string

	// Set AgentPermission 设置特定代理权限.
	SetAgentPermission(perm *AgentPermission) error

	// Get AgentPermission获得特定代理权限.
	GetAgentPermission(agentID string) (*AgentPermission, bool)
}

// 默认许可管理器是权限管理器的默认执行.
type DefaultPermissionManager struct {
	rules            map[string]*PermissionRule
	roles            map[string]*Role
	userRoles        map[string][]string // userID -> roleIDs
	agentPermissions map[string]*AgentPermission
	approvalHandler  ApprovalHandler
	logger           *zap.Logger
	mu               sync.RWMutex
}

// 审批人处理要求审批申请 审批决定.
type ApprovalHandler interface {
	RequestApproval(ctx context.Context, permCtx *PermissionContext, rule *PermissionRule) (approvalID string, err error)
	CheckApprovalStatus(ctx context.Context, approvalID string) (approved bool, err error)
}

// NewPermissionManager创建了新的许可管理器.
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

// SetApprovalHandler设定审批处理器.
func (pm *DefaultPermissionManager) SetApprovalHandler(handler ApprovalHandler) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.approvalHandler = handler
}

// 检查是否允许调用工具 。
func (pm *DefaultPermissionManager) CheckPermission(ctx context.Context, permCtx *PermissionContext) (*PermissionCheckResult, error) {
	start := time.Now()
	result := &PermissionCheckResult{
		CheckedAt: start,
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// 1. 先检查代理特定权限
	if agentPerm, ok := pm.agentPermissions[permCtx.AgentID]; ok {
		decision := pm.checkAgentPermission(agentPerm, permCtx.ToolName)
		if decision != "" {
			result.Decision = decision
			result.Reason = fmt.Sprintf("agent-specific permission: %s", decision)
			result.CheckLatency = time.Since(start)
			return result, nil
		}
	}

	// 2. 收集所有适用角色(包括继承角色)
	allRoles := pm.collectRoles(permCtx.UserID, permCtx.Roles)

	// 3. 收集所有适用规则
	applicableRules := pm.collectApplicableRules(permCtx.ToolName, allRoles)

	// 4. 按优先权排序规则(优先排序较高)
	sortRulesByPriority(applicableRules)

	// 5. 评价规则
	for _, rule := range applicableRules {
		if pm.evaluateRule(rule, permCtx) {
			result.Decision = rule.Decision
			result.MatchedRule = rule
			result.Reason = fmt.Sprintf("matched rule: %s", rule.Name)

			// 处理需要  批准
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

	// 6. 默认:如果没有规则匹配则拒绝
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

// 请检查access-date=中的日期值 (帮助)
func (pm *DefaultPermissionManager) checkAgentPermission(perm *AgentPermission, toolName string) PermissionDecision {
	// 先检查明确拒绝列表
	for _, denied := range perm.DeniedTools {
		if matchPattern(denied, toolName) {
			return PermissionDeny
		}
	}

	// 检查明确的允许列表
	for _, allowed := range perm.AllowedTools {
		if matchPattern(allowed, toolName) {
			return PermissionAllow
		}
	}

	// 检查继承的权限
	if perm.InheritFrom != "" {
		if parentPerm, ok := pm.agentPermissions[perm.InheritFrom]; ok {
			return pm.checkAgentPermission(parentPerm, toolName)
		}
	}

	return "" // No decision, continue to other checks
}

// 收集Roles收集所有适用的角色,包括继承的角色。
func (pm *DefaultPermissionManager) collectRoles(userID string, explicitRoles []string) []string {
	roleSet := make(map[string]bool)

	// 添加明确的角色
	for _, r := range explicitRoles {
		roleSet[r] = true
	}

	// 添加用户指定的角色
	if userRoles, ok := pm.userRoles[userID]; ok {
		for _, r := range userRoles {
			roleSet[r] = true
		}
	}

	// 扩大继承角色
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

// 收集可应用的规则收集适用于工具和角色的所有规则。
func (pm *DefaultPermissionManager) collectApplicableRules(toolName string, roles []string) []*PermissionRule {
	var rules []*PermissionRule

	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r] = true
	}

	for _, rule := range pm.rules {
		// 检查工具模式是否匹配
		if !matchPattern(rule.ToolPattern, toolName) {
			continue
		}

		// 检查时间有效性
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

// 依据许可上下文评价规则。
func (pm *DefaultPermissionManager) evaluateRule(rule *PermissionRule, permCtx *PermissionContext) bool {
	// 评估所有条件
	for _, cond := range rule.Conditions {
		if !pm.evaluateCondition(cond, permCtx) {
			return false
		}
	}
	return true
}

// 评估条件评价单一条件。
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

// 添加规则添加了权限规则 。
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

// 删除规则删除权限规则 。
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

// Get Rule 以 ID 检索许可规则 。
func (pm *DefaultPermissionManager) GetRule(ruleID string) (*PermissionRule, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	rule, ok := pm.rules[ruleID]
	return rule, ok
}

// List Rules 列出所有许可规则 。
func (pm *DefaultPermissionManager) ListRules() []*PermissionRule {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rules := make([]*PermissionRule, 0, len(pm.rules))
	for _, rule := range pm.rules {
		rules = append(rules, rule)
	}
	return rules
}

// 添加Role增加了一个角色.
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

// 删除 Role 删除一个角色 。
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

// GetRole通过ID检索角色.
func (pm *DefaultPermissionManager) GetRole(roleID string) (*Role, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	role, ok := pm.roles[roleID]
	return role, ok
}

// AssignRoleToUser为用户分配一个角色.
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

// GetUserRoles为用户获得所有角色.
func (pm *DefaultPermissionManager) GetUserRoles(userID string) []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return append([]string{}, pm.userRoles[userID]...)
}

// Set AgentPermission 设置特定代理权限.
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

// Get AgentPermission获得特定代理权限.
func (pm *DefaultPermissionManager) GetAgentPermission(agentID string) (*AgentPermission, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	perm, ok := pm.agentPermissions[agentID]
	return perm, ok
}

// 辅助功能

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
	// 简单的通配符匹配
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

// CreatyMiddleware 创建了一个在工具执行前检查权限的中间软件.
func PermissionMiddleware(pm PermissionManager) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// 从上下文提取权限上下文
			permCtx, ok := ctx.Value(permissionContextKey).(*PermissionContext)
			if !ok {
				return nil, fmt.Errorf("permission context not found")
			}

			// 检查权限
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

// With PermissionContext 为上下文添加许可上下文 。
func WithPermissionContext(ctx context.Context, permCtx *PermissionContext) context.Context {
	return context.WithValue(ctx, permissionContextKey, permCtx)
}

// Get PermissionContext 从上下文检索权限上下文 。
func GetPermissionContext(ctx context.Context) (*PermissionContext, bool) {
	permCtx, ok := ctx.Value(permissionContextKey).(*PermissionContext)
	return permCtx, ok
}

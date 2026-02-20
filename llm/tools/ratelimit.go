// 包工具为企业AI Agent框架中的工具执行提供了费率限制.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// rateLimit战略定义了限速策略.
type RateLimitStrategy string

const (
	RateLimitStrategySlidingWindow RateLimitStrategy = "sliding_window"
	RateLimitStrategyTokenBucket   RateLimitStrategy = "token_bucket"
	RateLimitStrategyFixedWindow   RateLimitStrategy = "fixed_window"
)

// PrateLimit Action 定义了超过利率限制时要采取的行动.
type RateLimitAction string

const (
	RateLimitActionReject  RateLimitAction = "reject"  // Reject the request immediately
	RateLimitActionQueue   RateLimitAction = "queue"   // Queue the request for later execution
	RateLimitActionDegrade RateLimitAction = "degrade" // Degrade service (e.g., use cached response)
)

// rateLimitScope定义了限制费率的范围.
type RateLimitScope string

const (
	RateLimitScopeGlobal  RateLimitScope = "global"   // Global rate limit
	RateLimitScopeTool    RateLimitScope = "tool"     // Per-tool rate limit
	RateLimitScopeAgent   RateLimitScope = "agent"    // Per-agent rate limit
	RateLimitScopeUser    RateLimitScope = "user"     // Per-user rate limit
	RateLimitScopeSession RateLimitScope = "session"  // Per-session rate limit
)

// LeaderLimit规则定义了费率限制规则.
type RateLimitRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Scope       RateLimitScope    `json:"scope"`
	Strategy    RateLimitStrategy `json:"strategy"`
	ToolPattern string            `json:"tool_pattern,omitempty"` // For tool-specific limits
	MaxRequests int               `json:"max_requests"`           // Maximum requests allowed
	Window      time.Duration     `json:"window"`                 // Time window
	BurstSize   int               `json:"burst_size,omitempty"`   // For token bucket
	RefillRate  float64           `json:"refill_rate,omitempty"`  // Tokens per second for token bucket
	Action      RateLimitAction   `json:"action"`                 // Action when limit exceeded
	Priority    int               `json:"priority"`               // Higher priority rules are checked first
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// LeaderLimitContext为费率限制检查提供了上下文.
type RateLimitContext struct {
	AgentID   string `json:"agent_id"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id,omitempty"`
	ToolName  string `json:"tool_name"`
	RequestAt time.Time `json:"request_at"`
}

// LeaderLimitResult包含一个速率限制检查的结果.
type RateLimitResult struct {
	Allowed       bool            `json:"allowed"`
	Rule          *RateLimitRule  `json:"rule,omitempty"`
	Action        RateLimitAction `json:"action,omitempty"`
	RetryAfter    time.Duration   `json:"retry_after,omitempty"`
	RemainingCalls int            `json:"remaining_calls"`
	ResetAt       time.Time       `json:"reset_at,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

// rateLimitManager管理限制费率.
type RateLimitManager interface {
	// 请检查是否允许请求 。
	CheckRateLimit(ctx context.Context, rlCtx *RateLimitContext) (*RateLimitResult, error)

	// 添加规则添加了费率限制规则 。
	AddRule(rule *RateLimitRule) error

	// 删除规则删除利率限制规则。
	RemoveRule(ruleID string) error

	// Get Rule 以 ID 检索费率限制规则 。
	GetRule(ruleID string) (*RateLimitRule, bool)

	// 列表规则列出所有费率限制规则 。
	ListRules() []*RateLimitRule

	// GetStats 返回率限制统计.
	GetStats(scope RateLimitScope, key string) *RateLimitStats

	// 重置特定密钥的速率限制计数器 。
	Reset(scope RateLimitScope, key string) error
}

// rateLimitStats包含速率限制统计.
type RateLimitStats struct {
	Scope          RateLimitScope `json:"scope"`
	Key            string         `json:"key"`
	TotalRequests  int64          `json:"total_requests"`
	AllowedRequests int64         `json:"allowed_requests"`
	RejectedRequests int64        `json:"rejected_requests"`
	CurrentCount   int            `json:"current_count"`
	WindowStart    time.Time      `json:"window_start"`
	WindowEnd      time.Time      `json:"window_end"`
}

// 默认RateLimitManager是RateLimitManager的默认执行.
type DefaultRateLimitManager struct {
	rules          map[string]*RateLimitRule
	limiters       map[string]Limiter // key -> limiter
	stats          map[string]*RateLimitStats
	queueHandler   QueueHandler
	degradeHandler DegradeHandler
	logger         *zap.Logger
	mu             sync.RWMutex
}

// 限制器定义了限速器的接口.
type Limiter interface {
	Allow() bool
	Remaining() int
	ResetAt() time.Time
	Reset()
}

// QueHandler 处理已排队的请求 。
type QueueHandler interface {
	Enqueue(ctx context.Context, rlCtx *RateLimitContext) error
	Dequeue(ctx context.Context) (*RateLimitContext, error)
}

// 降解汉德勒处理退化的服务.
type DegradeHandler interface {
	GetDegradedResponse(ctx context.Context, rlCtx *RateLimitContext) (json.RawMessage, error)
}

// NewRateLimitManager创建了新的费率限制管理器.
func NewRateLimitManager(logger *zap.Logger) *DefaultRateLimitManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DefaultRateLimitManager{
		rules:    make(map[string]*RateLimitRule),
		limiters: make(map[string]Limiter),
		stats:    make(map[string]*RateLimitStats),
		logger:   logger.With(zap.String("component", "rate_limit_manager")),
	}
}

// 设置 QueeHandler 设置队列处理器 。
func (rlm *DefaultRateLimitManager) SetQueueHandler(handler QueueHandler) {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()
	rlm.queueHandler = handler
}

// 设置 DegradeHandler 设置可降解处理器 。
func (rlm *DefaultRateLimitManager) SetDegradeHandler(handler DegradeHandler) {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()
	rlm.degradeHandler = handler
}

// 请检查是否允许请求 。
func (rlm *DefaultRateLimitManager) CheckRateLimit(ctx context.Context, rlCtx *RateLimitContext) (*RateLimitResult, error) {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	result := &RateLimitResult{
		Allowed: true,
	}

	// 查找按优先权排序的适用规则
	applicableRules := rlm.findApplicableRules(rlCtx)

	for _, rule := range applicableRules {
		if !rule.Enabled {
			continue
		}

		key := rlm.buildLimiterKey(rule, rlCtx)
		limiter := rlm.getOrCreateLimiter(key, rule)

		if !limiter.Allow() {
			result.Allowed = false
			result.Rule = rule
			result.Action = rule.Action
			result.RetryAfter = time.Until(limiter.ResetAt())
			result.ResetAt = limiter.ResetAt()
			result.Reason = fmt.Sprintf("rate limit exceeded for rule: %s", rule.Name)

			// 更新数据
			rlm.updateStats(key, false)

			rlm.logger.Warn("rate limit exceeded",
				zap.String("rule_id", rule.ID),
				zap.String("key", key),
				zap.Duration("retry_after", result.RetryAfter),
			)

			return result, nil
		}

		result.RemainingCalls = limiter.Remaining()
		result.ResetAt = limiter.ResetAt()

		// 更新数据
		rlm.updateStats(key, true)
	}

	return result, nil
}

// find Applicable Rules 查找适用于上下文的规则。
func (rlm *DefaultRateLimitManager) findApplicableRules(rlCtx *RateLimitContext) []*RateLimitRule {
	var rules []*RateLimitRule

	for _, rule := range rlm.rules {
		if rlm.ruleApplies(rule, rlCtx) {
			rules = append(rules, rule)
		}
	}

	// 按优先排序( 先高一点)
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}

	return rules
}

// 使用规则检查规则是否适用于上下文。
func (rlm *DefaultRateLimitManager) ruleApplies(rule *RateLimitRule, rlCtx *RateLimitContext) bool {
	// 检查工具模式
	if rule.ToolPattern != "" && !matchPattern(rule.ToolPattern, rlCtx.ToolName) {
		return false
	}

	// 检查范围
	switch rule.Scope {
	case RateLimitScopeGlobal:
		return true
	case RateLimitScopeTool:
		return rlCtx.ToolName != ""
	case RateLimitScopeAgent:
		return rlCtx.AgentID != ""
	case RateLimitScopeUser:
		return rlCtx.UserID != ""
	case RateLimitScopeSession:
		return rlCtx.SessionID != ""
	}

	return true
}

// 构建 LimiterKey 为限制器构建一个独特的密钥 。
func (rlm *DefaultRateLimitManager) buildLimiterKey(rule *RateLimitRule, rlCtx *RateLimitContext) string {
	switch rule.Scope {
	case RateLimitScopeGlobal:
		return fmt.Sprintf("global:%s", rule.ID)
	case RateLimitScopeTool:
		return fmt.Sprintf("tool:%s:%s", rule.ID, rlCtx.ToolName)
	case RateLimitScopeAgent:
		return fmt.Sprintf("agent:%s:%s", rule.ID, rlCtx.AgentID)
	case RateLimitScopeUser:
		return fmt.Sprintf("user:%s:%s", rule.ID, rlCtx.UserID)
	case RateLimitScopeSession:
		return fmt.Sprintf("session:%s:%s", rule.ID, rlCtx.SessionID)
	}
	return fmt.Sprintf("unknown:%s", rule.ID)
}

// 获得 OrCreateLimiter 键得到或创建限制键。
func (rlm *DefaultRateLimitManager) getOrCreateLimiter(key string, rule *RateLimitRule) Limiter {
	if limiter, ok := rlm.limiters[key]; ok {
		return limiter
	}

	var limiter Limiter
	switch rule.Strategy {
	case RateLimitStrategyTokenBucket:
		limiter = NewTokenBucketLimiter(rule.BurstSize, rule.RefillRate)
	case RateLimitStrategyFixedWindow:
		limiter = NewFixedWindowLimiter(rule.MaxRequests, rule.Window)
	default: // sliding window
		limiter = NewSlidingWindowLimiter(rule.MaxRequests, rule.Window)
	}

	rlm.limiters[key] = limiter
	return limiter
}

// 更新 Stats 更新率限制统计.
func (rlm *DefaultRateLimitManager) updateStats(key string, allowed bool) {
	stats, ok := rlm.stats[key]
	if !ok {
		stats = &RateLimitStats{
			Key:         key,
			WindowStart: time.Now(),
		}
		rlm.stats[key] = stats
	}

	stats.TotalRequests++
	if allowed {
		stats.AllowedRequests++
	} else {
		stats.RejectedRequests++
	}
}

// 添加规则添加了费率限制规则 。
func (rlm *DefaultRateLimitManager) AddRule(rule *RateLimitRule) error {
	if rule.ID == "" {
		return fmt.Errorf("rule ID is required")
	}

	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	rule.CreatedAt = time.Now()
	rule.UpdatedAt = time.Now()
	rlm.rules[rule.ID] = rule

	rlm.logger.Info("rate limit rule added",
		zap.String("rule_id", rule.ID),
		zap.String("name", rule.Name),
		zap.String("scope", string(rule.Scope)),
	)

	return nil
}

// 删除规则删除利率限制规则。
func (rlm *DefaultRateLimitManager) RemoveRule(ruleID string) error {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	if _, ok := rlm.rules[ruleID]; !ok {
		return fmt.Errorf("rule not found: %s", ruleID)
	}

	delete(rlm.rules, ruleID)
	rlm.logger.Info("rate limit rule removed", zap.String("rule_id", ruleID))
	return nil
}

// Get Rule 以 ID 检索费率限制规则 。
func (rlm *DefaultRateLimitManager) GetRule(ruleID string) (*RateLimitRule, bool) {
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()
	rule, ok := rlm.rules[ruleID]
	return rule, ok
}

// 列表规则列出所有费率限制规则 。
func (rlm *DefaultRateLimitManager) ListRules() []*RateLimitRule {
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()

	rules := make([]*RateLimitRule, 0, len(rlm.rules))
	for _, rule := range rlm.rules {
		rules = append(rules, rule)
	}
	return rules
}

// GetStats 返回率限制统计.
func (rlm *DefaultRateLimitManager) GetStats(scope RateLimitScope, key string) *RateLimitStats {
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()

	fullKey := fmt.Sprintf("%s:%s", scope, key)
	return rlm.stats[fullKey]
}

// 重置特定密钥的速率限制计数器 。
func (rlm *DefaultRateLimitManager) Reset(scope RateLimitScope, key string) error {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	fullKey := fmt.Sprintf("%s:%s", scope, key)
	if limiter, ok := rlm.limiters[fullKey]; ok {
		limiter.Reset()
	}
	delete(rlm.stats, fullKey)

	rlm.logger.Info("rate limit reset",
		zap.String("scope", string(scope)),
		zap.String("key", key),
	)

	return nil
}

// * 滑动窗口限制器 * *

// 滑动WindowLimiter执行滑动窗口速率限制.
type SlidingWindowLimiter struct {
	maxRequests int
	window      time.Duration
	requests    []time.Time
	mu          sync.Mutex
}

// NewSliding WindowLimiter创建了新的滑动窗口限制器.
func NewSlidingWindowLimiter(maxRequests int, window time.Duration) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		maxRequests: maxRequests,
		window:      window,
		requests:    make([]time.Time, 0),
	}
}

// 允许检查请求是否被允许 。
func (l *SlidingWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// 删除已过期的请求
	validRequests := make([]time.Time, 0)
	for _, t := range l.requests {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}
	l.requests = validRequests

	// 检查是否超过了限制
	if len(l.requests) >= l.maxRequests {
		return false
	}

	// 记录此请求
	l.requests = append(l.requests, now)
	return true
}

// 剩余请求数返回 。
func (l *SlidingWindowLimiter) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	count := 0
	for _, t := range l.requests {
		if t.After(cutoff) {
			count++
		}
	}

	remaining := l.maxRequests - count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// 重置时返回 。
func (l *SlidingWindowLimiter) ResetAt() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.requests) == 0 {
		return time.Now()
	}

	// 在窗口中查找最老的请求
	oldest := l.requests[0]
	return oldest.Add(l.window)
}

// 重置限制器 。
func (l *SlidingWindowLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = make([]time.Time, 0)
}

// 键盘限制器

// TokenBucketLimiter 执行符号桶率限制 。
type TokenBucketLimiter struct {
	bucketSize  int
	refillRate  float64 // tokens per second
	tokens      float64
	lastRefill  time.Time
	mu          sync.Mutex
}

// NewTokenBucketLimiter 创建了新的符号桶限制器.
func NewTokenBucketLimiter(bucketSize int, refillRate float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		bucketSize: bucketSize,
		refillRate: refillRate,
		tokens:     float64(bucketSize),
		lastRefill: time.Now(),
	}
}

// 允许检查请求是否被允许 。
func (l *TokenBucketLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens < 1 {
		return false
	}

	l.tokens--
	return true
}

// 重新填充根据已过期时间添加符号 。
func (l *TokenBucketLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.tokens += elapsed * l.refillRate

	if l.tokens > float64(l.bucketSize) {
		l.tokens = float64(l.bucketSize)
	}

	l.lastRefill = now
}

// 剩余符号返回所剩符号数 。
func (l *TokenBucketLimiter) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	return int(l.tokens)
}

// 重置 Att 返回时桶将满 。
func (l *TokenBucketLimiter) ResetAt() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()

	if l.tokens >= float64(l.bucketSize) {
		return time.Now()
	}

	tokensNeeded := float64(l.bucketSize) - l.tokens
	secondsNeeded := tokensNeeded / l.refillRate
	return time.Now().Add(time.Duration(secondsNeeded * float64(time.Second)))
}

// 重置限制器 。
func (l *TokenBucketLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tokens = float64(l.bucketSize)
	l.lastRefill = time.Now()
}

// 固定窗口限制器

// FixtWindowLimiter执行固定窗口率限制.
type FixedWindowLimiter struct {
	maxRequests int
	window      time.Duration
	count       int
	windowStart time.Time
	mu          sync.Mutex
}

// 新Fixed WindowLimiter创建了新的固定窗口限制器.
func NewFixedWindowLimiter(maxRequests int, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		maxRequests: maxRequests,
		window:      window,
		windowStart: time.Now(),
	}
}

// 允许检查请求是否被允许 。
func (l *FixedWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// 检查窗口是否过期
	if now.Sub(l.windowStart) >= l.window {
		l.windowStart = now
		l.count = 0
	}

	// 检查是否超过了限制
	if l.count >= l.maxRequests {
		return false
	}

	l.count++
	return true
}

// 剩余请求数返回 。
func (l *FixedWindowLimiter) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// 检查窗口是否过期
	if now.Sub(l.windowStart) >= l.window {
		return l.maxRequests
	}

	remaining := l.maxRequests - l.count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// 重置At 返回窗口重置时 。
func (l *FixedWindowLimiter) ResetAt() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.windowStart.Add(l.window)
}

// 重置限制器 。
func (l *FixedWindowLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count = 0
	l.windowStart = time.Now()
}

// QQ 速率限制中件QQ

// rateLimitMiddleware创建了一个执行速率限制的中间软件.
func RateLimitMiddleware(rlm RateLimitManager, auditLogger AuditLogger) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// 摘录上下文信息
			permCtx, _ := GetPermissionContext(ctx)

			rlCtx := &RateLimitContext{
				RequestAt: time.Now(),
			}

			if permCtx != nil {
				rlCtx.AgentID = permCtx.AgentID
				rlCtx.UserID = permCtx.UserID
				rlCtx.SessionID = permCtx.SessionID
				rlCtx.ToolName = permCtx.ToolName
			}

			// 检查率限制
			result, err := rlm.CheckRateLimit(ctx, rlCtx)
			if err != nil {
				return nil, fmt.Errorf("rate limit check failed: %w", err)
			}

			if !result.Allowed {
				// 日志速率限制
				if auditLogger != nil {
					LogRateLimitHit(auditLogger, rlCtx.AgentID, rlCtx.UserID, rlCtx.ToolName, string(result.Rule.Scope))
				}

				switch result.Action {
				case RateLimitActionReject:
					return nil, fmt.Errorf("rate limit exceeded: %s (retry after %s)", result.Reason, result.RetryAfter)
				case RateLimitActionQueue:
					// 队列处理将在此执行
					return nil, fmt.Errorf("request queued due to rate limit: %s", result.Reason)
				case RateLimitActionDegrade:
					// 这里将实行有辱人格的处理
					return nil, fmt.Errorf("service degraded due to rate limit: %s", result.Reason)
				}
			}

			return next(ctx, args)
		}
	}
}

// • 方便功能

// 创建 GlobalRateLimit 创建全局速率限制规则 。
func CreateGlobalRateLimit(id, name string, maxRequests int, window time.Duration) *RateLimitRule {
	return &RateLimitRule{
		ID:          id,
		Name:        name,
		Scope:       RateLimitScopeGlobal,
		Strategy:    RateLimitStrategySlidingWindow,
		MaxRequests: maxRequests,
		Window:      window,
		Action:      RateLimitActionReject,
		Enabled:     true,
	}
}

// CreateTooRateLimit 创建了每工具速率限制规则.
func CreateToolRateLimit(id, name, toolPattern string, maxRequests int, window time.Duration) *RateLimitRule {
	return &RateLimitRule{
		ID:          id,
		Name:        name,
		Scope:       RateLimitScopeTool,
		Strategy:    RateLimitStrategySlidingWindow,
		ToolPattern: toolPattern,
		MaxRequests: maxRequests,
		Window:      window,
		Action:      RateLimitActionReject,
		Enabled:     true,
	}
}

// CreateUserRateLimit 创建了每个用户率限制规则.
func CreateUserRateLimit(id, name string, maxRequests int, window time.Duration) *RateLimitRule {
	return &RateLimitRule{
		ID:          id,
		Name:        name,
		Scope:       RateLimitScopeUser,
		Strategy:    RateLimitStrategySlidingWindow,
		MaxRequests: maxRequests,
		Window:      window,
		Action:      RateLimitActionReject,
		Enabled:     true,
	}
}

// 创建 AgentRateLimit 创建每代理费率限制规则。
func CreateAgentRateLimit(id, name string, maxRequests int, window time.Duration) *RateLimitRule {
	return &RateLimitRule{
		ID:          id,
		Name:        name,
		Scope:       RateLimitScopeAgent,
		Strategy:    RateLimitStrategySlidingWindow,
		MaxRequests: maxRequests,
		Window:      window,
		Action:      RateLimitActionReject,
		Enabled:     true,
	}
}

// CreateTokenBucketRateLimit 创建了符号桶速率限制规则 。
func CreateTokenBucketRateLimit(id, name string, bucketSize int, refillRate float64) *RateLimitRule {
	return &RateLimitRule{
		ID:         id,
		Name:       name,
		Scope:      RateLimitScopeGlobal,
		Strategy:   RateLimitStrategyTokenBucket,
		BurstSize:  bucketSize,
		RefillRate: refillRate,
		Action:     RateLimitActionReject,
		Enabled:    true,
	}
}

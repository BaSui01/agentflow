// Package tools provides rate limiting for tool execution in enterprise AI Agent frameworks.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RateLimitStrategy defines the rate limiting strategy.
type RateLimitStrategy string

const (
	RateLimitStrategySlidingWindow RateLimitStrategy = "sliding_window"
	RateLimitStrategyTokenBucket   RateLimitStrategy = "token_bucket"
	RateLimitStrategyFixedWindow   RateLimitStrategy = "fixed_window"
)

// RateLimitAction defines the action to take when rate limit is exceeded.
type RateLimitAction string

const (
	RateLimitActionReject  RateLimitAction = "reject"  // Reject the request immediately
	RateLimitActionQueue   RateLimitAction = "queue"   // Queue the request for later execution
	RateLimitActionDegrade RateLimitAction = "degrade" // Degrade service (e.g., use cached response)
)

// RateLimitScope defines the scope of rate limiting.
type RateLimitScope string

const (
	RateLimitScopeGlobal  RateLimitScope = "global"   // Global rate limit
	RateLimitScopeTool    RateLimitScope = "tool"     // Per-tool rate limit
	RateLimitScopeAgent   RateLimitScope = "agent"    // Per-agent rate limit
	RateLimitScopeUser    RateLimitScope = "user"     // Per-user rate limit
	RateLimitScopeSession RateLimitScope = "session"  // Per-session rate limit
)

// RateLimitRule defines a rate limit rule.
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

// RateLimitContext provides context for rate limit checks.
type RateLimitContext struct {
	AgentID   string `json:"agent_id"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id,omitempty"`
	ToolName  string `json:"tool_name"`
	RequestAt time.Time `json:"request_at"`
}

// RateLimitResult contains the result of a rate limit check.
type RateLimitResult struct {
	Allowed       bool            `json:"allowed"`
	Rule          *RateLimitRule  `json:"rule,omitempty"`
	Action        RateLimitAction `json:"action,omitempty"`
	RetryAfter    time.Duration   `json:"retry_after,omitempty"`
	RemainingCalls int            `json:"remaining_calls"`
	ResetAt       time.Time       `json:"reset_at,omitempty"`
	Reason        string          `json:"reason,omitempty"`
}

// RateLimitManager manages rate limiting.
type RateLimitManager interface {
	// CheckRateLimit checks if a request is allowed.
	CheckRateLimit(ctx context.Context, rlCtx *RateLimitContext) (*RateLimitResult, error)

	// AddRule adds a rate limit rule.
	AddRule(rule *RateLimitRule) error

	// RemoveRule removes a rate limit rule.
	RemoveRule(ruleID string) error

	// GetRule retrieves a rate limit rule by ID.
	GetRule(ruleID string) (*RateLimitRule, bool)

	// ListRules lists all rate limit rules.
	ListRules() []*RateLimitRule

	// GetStats returns rate limit statistics.
	GetStats(scope RateLimitScope, key string) *RateLimitStats

	// Reset resets rate limit counters for a specific key.
	Reset(scope RateLimitScope, key string) error
}

// RateLimitStats contains rate limit statistics.
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

// DefaultRateLimitManager is the default implementation of RateLimitManager.
type DefaultRateLimitManager struct {
	rules          map[string]*RateLimitRule
	limiters       map[string]Limiter // key -> limiter
	stats          map[string]*RateLimitStats
	queueHandler   QueueHandler
	degradeHandler DegradeHandler
	logger         *zap.Logger
	mu             sync.RWMutex
}

// Limiter defines the interface for rate limiters.
type Limiter interface {
	Allow() bool
	Remaining() int
	ResetAt() time.Time
	Reset()
}

// QueueHandler handles queued requests.
type QueueHandler interface {
	Enqueue(ctx context.Context, rlCtx *RateLimitContext) error
	Dequeue(ctx context.Context) (*RateLimitContext, error)
}

// DegradeHandler handles degraded service.
type DegradeHandler interface {
	GetDegradedResponse(ctx context.Context, rlCtx *RateLimitContext) (json.RawMessage, error)
}

// NewRateLimitManager creates a new rate limit manager.
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

// SetQueueHandler sets the queue handler.
func (rlm *DefaultRateLimitManager) SetQueueHandler(handler QueueHandler) {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()
	rlm.queueHandler = handler
}

// SetDegradeHandler sets the degrade handler.
func (rlm *DefaultRateLimitManager) SetDegradeHandler(handler DegradeHandler) {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()
	rlm.degradeHandler = handler
}

// CheckRateLimit checks if a request is allowed.
func (rlm *DefaultRateLimitManager) CheckRateLimit(ctx context.Context, rlCtx *RateLimitContext) (*RateLimitResult, error) {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	result := &RateLimitResult{
		Allowed: true,
	}

	// Find applicable rules sorted by priority
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

			// Update stats
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

		// Update stats
		rlm.updateStats(key, true)
	}

	return result, nil
}

// findApplicableRules finds rules applicable to the context.
func (rlm *DefaultRateLimitManager) findApplicableRules(rlCtx *RateLimitContext) []*RateLimitRule {
	var rules []*RateLimitRule

	for _, rule := range rlm.rules {
		if rlm.ruleApplies(rule, rlCtx) {
			rules = append(rules, rule)
		}
	}

	// Sort by priority (higher first)
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}

	return rules
}

// ruleApplies checks if a rule applies to the context.
func (rlm *DefaultRateLimitManager) ruleApplies(rule *RateLimitRule, rlCtx *RateLimitContext) bool {
	// Check tool pattern
	if rule.ToolPattern != "" && !matchPattern(rule.ToolPattern, rlCtx.ToolName) {
		return false
	}

	// Check scope
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

// buildLimiterKey builds a unique key for the limiter.
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

// getOrCreateLimiter gets or creates a limiter for the key.
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

// updateStats updates rate limit statistics.
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

// AddRule adds a rate limit rule.
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

// RemoveRule removes a rate limit rule.
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

// GetRule retrieves a rate limit rule by ID.
func (rlm *DefaultRateLimitManager) GetRule(ruleID string) (*RateLimitRule, bool) {
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()
	rule, ok := rlm.rules[ruleID]
	return rule, ok
}

// ListRules lists all rate limit rules.
func (rlm *DefaultRateLimitManager) ListRules() []*RateLimitRule {
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()

	rules := make([]*RateLimitRule, 0, len(rlm.rules))
	for _, rule := range rlm.rules {
		rules = append(rules, rule)
	}
	return rules
}

// GetStats returns rate limit statistics.
func (rlm *DefaultRateLimitManager) GetStats(scope RateLimitScope, key string) *RateLimitStats {
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()

	fullKey := fmt.Sprintf("%s:%s", scope, key)
	return rlm.stats[fullKey]
}

// Reset resets rate limit counters for a specific key.
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

// ====== Sliding Window Limiter ======

// SlidingWindowLimiter implements sliding window rate limiting.
type SlidingWindowLimiter struct {
	maxRequests int
	window      time.Duration
	requests    []time.Time
	mu          sync.Mutex
}

// NewSlidingWindowLimiter creates a new sliding window limiter.
func NewSlidingWindowLimiter(maxRequests int, window time.Duration) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		maxRequests: maxRequests,
		window:      window,
		requests:    make([]time.Time, 0),
	}
}

// Allow checks if a request is allowed.
func (l *SlidingWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-l.window)

	// Remove expired requests
	validRequests := make([]time.Time, 0)
	for _, t := range l.requests {
		if t.After(cutoff) {
			validRequests = append(validRequests, t)
		}
	}
	l.requests = validRequests

	// Check if limit exceeded
	if len(l.requests) >= l.maxRequests {
		return false
	}

	// Record this request
	l.requests = append(l.requests, now)
	return true
}

// Remaining returns the number of remaining requests.
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

// ResetAt returns when the rate limit will reset.
func (l *SlidingWindowLimiter) ResetAt() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.requests) == 0 {
		return time.Now()
	}

	// Find the oldest request in the window
	oldest := l.requests[0]
	return oldest.Add(l.window)
}

// Reset resets the limiter.
func (l *SlidingWindowLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests = make([]time.Time, 0)
}

// ====== Token Bucket Limiter ======

// TokenBucketLimiter implements token bucket rate limiting.
type TokenBucketLimiter struct {
	bucketSize  int
	refillRate  float64 // tokens per second
	tokens      float64
	lastRefill  time.Time
	mu          sync.Mutex
}

// NewTokenBucketLimiter creates a new token bucket limiter.
func NewTokenBucketLimiter(bucketSize int, refillRate float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		bucketSize: bucketSize,
		refillRate: refillRate,
		tokens:     float64(bucketSize),
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed.
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

// refill adds tokens based on elapsed time.
func (l *TokenBucketLimiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.tokens += elapsed * l.refillRate

	if l.tokens > float64(l.bucketSize) {
		l.tokens = float64(l.bucketSize)
	}

	l.lastRefill = now
}

// Remaining returns the number of remaining tokens.
func (l *TokenBucketLimiter) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	return int(l.tokens)
}

// ResetAt returns when the bucket will be full.
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

// Reset resets the limiter.
func (l *TokenBucketLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tokens = float64(l.bucketSize)
	l.lastRefill = time.Now()
}

// ====== Fixed Window Limiter ======

// FixedWindowLimiter implements fixed window rate limiting.
type FixedWindowLimiter struct {
	maxRequests int
	window      time.Duration
	count       int
	windowStart time.Time
	mu          sync.Mutex
}

// NewFixedWindowLimiter creates a new fixed window limiter.
func NewFixedWindowLimiter(maxRequests int, window time.Duration) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		maxRequests: maxRequests,
		window:      window,
		windowStart: time.Now(),
	}
}

// Allow checks if a request is allowed.
func (l *FixedWindowLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Check if window has expired
	if now.Sub(l.windowStart) >= l.window {
		l.windowStart = now
		l.count = 0
	}

	// Check if limit exceeded
	if l.count >= l.maxRequests {
		return false
	}

	l.count++
	return true
}

// Remaining returns the number of remaining requests.
func (l *FixedWindowLimiter) Remaining() int {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Check if window has expired
	if now.Sub(l.windowStart) >= l.window {
		return l.maxRequests
	}

	remaining := l.maxRequests - l.count
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// ResetAt returns when the window will reset.
func (l *FixedWindowLimiter) ResetAt() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.windowStart.Add(l.window)
}

// Reset resets the limiter.
func (l *FixedWindowLimiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count = 0
	l.windowStart = time.Now()
}

// ====== Rate Limit Middleware ======

// RateLimitMiddleware creates a middleware that enforces rate limits.
func RateLimitMiddleware(rlm RateLimitManager, auditLogger AuditLogger) func(ToolFunc) ToolFunc {
	return func(next ToolFunc) ToolFunc {
		return func(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
			// Extract context information
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

			// Check rate limit
			result, err := rlm.CheckRateLimit(ctx, rlCtx)
			if err != nil {
				return nil, fmt.Errorf("rate limit check failed: %w", err)
			}

			if !result.Allowed {
				// Log rate limit hit
				if auditLogger != nil {
					LogRateLimitHit(auditLogger, rlCtx.AgentID, rlCtx.UserID, rlCtx.ToolName, string(result.Rule.Scope))
				}

				switch result.Action {
				case RateLimitActionReject:
					return nil, fmt.Errorf("rate limit exceeded: %s (retry after %s)", result.Reason, result.RetryAfter)
				case RateLimitActionQueue:
					// Queue handling would be implemented here
					return nil, fmt.Errorf("request queued due to rate limit: %s", result.Reason)
				case RateLimitActionDegrade:
					// Degrade handling would be implemented here
					return nil, fmt.Errorf("service degraded due to rate limit: %s", result.Reason)
				}
			}

			return next(ctx, args)
		}
	}
}

// ====== Convenience Functions ======

// CreateGlobalRateLimit creates a global rate limit rule.
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

// CreateToolRateLimit creates a per-tool rate limit rule.
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

// CreateUserRateLimit creates a per-user rate limit rule.
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

// CreateAgentRateLimit creates a per-agent rate limit rule.
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

// CreateTokenBucketRateLimit creates a token bucket rate limit rule.
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

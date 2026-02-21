package guardrails

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// OutputValidatorConfig 输出验证器配置
type OutputValidatorConfig struct {
	// Validators 输出验证器列表
	Validators []Validator
	// Filters 输出过滤器列表
	Filters []Filter
	// SafeReplacement 有害内容的安全替代响应
	SafeReplacement string
	// EnableAuditLog 是否启用审计日志
	EnableAuditLog bool
	// AuditLogger 审计日志记录器
	AuditLogger AuditLogger
	// Priority 验证器优先级
	Priority int
}

// DefaultOutputValidatorConfig 返回默认配置
func DefaultOutputValidatorConfig() *OutputValidatorConfig {
	return &OutputValidatorConfig{
		Validators:      []Validator{},
		Filters:         []Filter{},
		SafeReplacement: "[内容已被安全系统过滤]",
		EnableAuditLog:  true,
		AuditLogger:     nil,
		Priority:        50,
	}
}

// OutputValidator 输出验证器
// 用于验证 Agent 输出的安全性和合规性
// Requirements 2.1: 检测并脱敏敏感信息
// Requirements 2.2: 拦截有害内容并返回安全替代响应
// Requirements 2.3: 验证输出格式
// Requirements 2.5: 记录所有验证失败事件用于审计
type OutputValidator struct {
	validators      []Validator
	filters         []Filter
	safeReplacement string
	enableAuditLog  bool
	auditLogger     AuditLogger
	priority        int
	mu              sync.RWMutex
}

// NewOutputValidator 创建输出验证器
func NewOutputValidator(config *OutputValidatorConfig) *OutputValidator {
	if config == nil {
		config = DefaultOutputValidatorConfig()
	}

	// 复制验证器和过滤器列表
	validators := make([]Validator, len(config.Validators))
	copy(validators, config.Validators)

	filters := make([]Filter, len(config.Filters))
	copy(filters, config.Filters)

	auditLogger := config.AuditLogger
	if auditLogger == nil && config.EnableAuditLog {
		auditLogger = NewMemoryAuditLogger(1000)
	}

	return &OutputValidator{
		validators:      validators,
		filters:         filters,
		safeReplacement: config.SafeReplacement,
		enableAuditLog:  config.EnableAuditLog,
		auditLogger:     auditLogger,
		priority:        config.Priority,
	}
}

// Name 返回验证器名称
func (v *OutputValidator) Name() string {
	return "output_validator"
}

// Priority 返回优先级
func (v *OutputValidator) Priority() int {
	return v.priority
}

// Validate 执行输出验证
// 实现 Validator 接口
func (v *OutputValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	v.mu.RLock()
	validators := make([]Validator, len(v.validators))
	copy(validators, v.validators)
	enableAuditLog := v.enableAuditLog
	auditLogger := v.auditLogger
	v.mu.RUnlock()

	result := NewValidationResult()
	result.Metadata["output_validation"] = true

	// 按优先级排序验证器
	sortValidatorsByPriority(validators)

	// 执行所有验证器
	for _, validator := range validators {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		vResult, err := validator.Validate(ctx, content)
		if err != nil {
			result.AddError(ValidationError{
				Code:     ErrCodeValidationFailed,
				Message:  fmt.Sprintf("输出验证器 %s 执行失败: %s", validator.Name(), err.Error()),
				Severity: SeverityCritical,
			})
			continue
		}

		result.Merge(vResult)

		// 记录验证失败事件
		if enableAuditLog && auditLogger != nil && !vResult.Valid {
			v.logValidationFailure(ctx, auditLogger, validator.Name(), content, vResult)
		}
	}

	return result, nil
}

// ValidateAndFilter 验证并过滤输出内容
// 返回过滤后的内容和验证结果
func (v *OutputValidator) ValidateAndFilter(ctx context.Context, content string) (string, *ValidationResult, error) {
	// 首先执行验证
	result, err := v.Validate(ctx, content)
	if err != nil {
		return "", result, err
	}

	// 如果验证失败且包含严重错误，返回安全替代响应
	if !result.Valid && v.hasCriticalError(result) {
		v.mu.RLock()
		safeReplacement := v.safeReplacement
		v.mu.RUnlock()

		result.Metadata["replaced_with_safe_response"] = true
		return safeReplacement, result, nil
	}

	// 执行过滤器
	filteredContent, filterErr := v.applyFilters(ctx, content)
	if filterErr != nil {
		return content, result, filterErr
	}

	if filteredContent != content {
		result.Metadata["content_filtered"] = true
		result.Metadata["original_length"] = len(content)
		result.Metadata["filtered_length"] = len(filteredContent)
	}

	return filteredContent, result, nil
}

// applyFilters 应用所有过滤器
func (v *OutputValidator) applyFilters(ctx context.Context, content string) (string, error) {
	v.mu.RLock()
	filters := make([]Filter, len(v.filters))
	copy(filters, v.filters)
	v.mu.RUnlock()

	result := content
	for _, filter := range filters {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		filtered, err := filter.Filter(ctx, result)
		if err != nil {
			return result, fmt.Errorf("过滤器 %s 执行失败: %w", filter.Name(), err)
		}
		result = filtered
	}

	return result, nil
}

// hasCriticalError 检查是否包含严重错误
func (v *OutputValidator) hasCriticalError(result *ValidationResult) bool {
	for _, err := range result.Errors {
		if err.Severity == SeverityCritical || err.Severity == SeverityHigh {
			return true
		}
	}
	return false
}

// logValidationFailure 记录验证失败事件
func (v *OutputValidator) logValidationFailure(ctx context.Context, logger AuditLogger, validatorName, content string, result *ValidationResult) {
	entry := &AuditLogEntry{
		Timestamp:     time.Now(),
		EventType:     AuditEventValidationFailed,
		ValidatorName: validatorName,
		ContentHash:   hashContent(content),
		Errors:        result.Errors,
		Metadata:      result.Metadata,
	}
	logger.Log(ctx, entry)
}

// AddValidator 添加验证器
func (v *OutputValidator) AddValidator(validator Validator) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.validators = append(v.validators, validator)
}

// AddFilter 添加过滤器
func (v *OutputValidator) AddFilter(filter Filter) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.filters = append(v.filters, filter)
}

// SetSafeReplacement 设置安全替代响应
func (v *OutputValidator) SetSafeReplacement(replacement string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.safeReplacement = replacement
}

// GetAuditLogger 获取审计日志记录器
func (v *OutputValidator) GetAuditLogger() AuditLogger {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.auditLogger
}

// ContentFilterConfig 内容过滤器配置
type ContentFilterConfig struct {
	// BlockedPatterns 禁止的正则模式
	BlockedPatterns []string
	// Replacement 替换文本
	Replacement string
	// CaseSensitive 是否区分大小写
	CaseSensitive bool
}

// DefaultContentFilterConfig 返回默认配置
func DefaultContentFilterConfig() *ContentFilterConfig {
	return &ContentFilterConfig{
		BlockedPatterns: []string{},
		Replacement:     "[已过滤]",
		CaseSensitive:   false,
	}
}

// ContentFilter 内容过滤器
// 用于过滤输出中的有害内容
// Requirements 2.2: 拦截有害内容
type ContentFilter struct {
	blockedPatterns []*regexp.Regexp
	rawPatterns     []string
	replacement     string
	caseSensitive   bool
	mu              sync.RWMutex
}

// NewContentFilter 创建内容过滤器
func NewContentFilter(config *ContentFilterConfig) (*ContentFilter, error) {
	if config == nil {
		config = DefaultContentFilterConfig()
	}

	filter := &ContentFilter{
		blockedPatterns: make([]*regexp.Regexp, 0),
		rawPatterns:     make([]string, 0),
		replacement:     config.Replacement,
		caseSensitive:   config.CaseSensitive,
	}

	// 编译正则模式
	for _, pattern := range config.BlockedPatterns {
		if err := filter.AddPattern(pattern); err != nil {
			return nil, fmt.Errorf("无效的正则模式 '%s': %w", pattern, err)
		}
	}

	return filter, nil
}

// Name 返回过滤器名称
func (f *ContentFilter) Name() string {
	return "content_filter"
}

// Filter 执行内容过滤
// 实现 Filter 接口
func (f *ContentFilter) Filter(ctx context.Context, content string) (string, error) {
	f.mu.RLock()
	patterns := make([]*regexp.Regexp, len(f.blockedPatterns))
	copy(patterns, f.blockedPatterns)
	replacement := f.replacement
	f.mu.RUnlock()

	result := content
	for _, pattern := range patterns {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		result = pattern.ReplaceAllString(result, replacement)
	}

	return result, nil
}

// AddPattern 添加禁止模式
func (f *ContentFilter) AddPattern(pattern string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// 如果不区分大小写，添加 (?i) 前缀
	regexPattern := pattern
	if !f.caseSensitive && !strings.HasPrefix(pattern, "(?i)") {
		regexPattern = "(?i)" + pattern
	}

	compiled, err := regexp.Compile(regexPattern)
	if err != nil {
		return err
	}

	f.blockedPatterns = append(f.blockedPatterns, compiled)
	f.rawPatterns = append(f.rawPatterns, pattern)
	return nil
}

// RemovePattern 移除禁止模式
func (f *ContentFilter) RemovePattern(pattern string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i, p := range f.rawPatterns {
		if p == pattern {
			f.blockedPatterns = append(f.blockedPatterns[:i], f.blockedPatterns[i+1:]...)
			f.rawPatterns = append(f.rawPatterns[:i], f.rawPatterns[i+1:]...)
			return true
		}
	}
	return false
}

// GetPatterns 返回所有禁止模式
func (f *ContentFilter) GetPatterns() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make([]string, len(f.rawPatterns))
	copy(result, f.rawPatterns)
	return result
}

// SetReplacement 设置替换文本
func (f *ContentFilter) SetReplacement(replacement string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replacement = replacement
}

// Detect 检测内容中是否包含禁止模式
func (f *ContentFilter) Detect(content string) []ContentMatch {
	f.mu.RLock()
	patterns := make([]*regexp.Regexp, len(f.blockedPatterns))
	copy(patterns, f.blockedPatterns)
	rawPatterns := make([]string, len(f.rawPatterns))
	copy(rawPatterns, f.rawPatterns)
	f.mu.RUnlock()

	var matches []ContentMatch
	for i, pattern := range patterns {
		locs := pattern.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			matches = append(matches, ContentMatch{
				Pattern:  rawPatterns[i],
				Value:    content[loc[0]:loc[1]],
				Position: loc[0],
				Length:   loc[1] - loc[0],
			})
		}
	}

	return matches
}

// ContentMatch 内容匹配结果
type ContentMatch struct {
	Pattern  string `json:"pattern"`
	Value    string `json:"value"`
	Position int    `json:"position"`
	Length   int    `json:"length"`
}

// ContentFilterValidator 内容过滤验证器
// 将 ContentFilter 包装为 Validator 接口
type ContentFilterValidator struct {
	filter   *ContentFilter
	priority int
}

// NewContentFilterValidator 创建内容过滤验证器
func NewContentFilterValidator(filter *ContentFilter, priority int) *ContentFilterValidator {
	return &ContentFilterValidator{
		filter:   filter,
		priority: priority,
	}
}

// Name 返回验证器名称
func (v *ContentFilterValidator) Name() string {
	return "content_filter_validator"
}

// Priority 返回优先级
func (v *ContentFilterValidator) Priority() int {
	return v.priority
}

// Validate 执行内容过滤验证
func (v *ContentFilterValidator) Validate(ctx context.Context, content string) (*ValidationResult, error) {
	result := NewValidationResult()

	matches := v.filter.Detect(content)
	if len(matches) == 0 {
		return result, nil
	}

	// 记录检测到的有害内容
	result.Metadata["blocked_content_detected"] = true
	result.Metadata["content_matches"] = matches
	result.Metadata["match_count"] = len(matches)

	// 添加错误
	result.AddError(ValidationError{
		Code:     ErrCodeContentBlocked,
		Message:  fmt.Sprintf("检测到 %d 处禁止内容", len(matches)),
		Severity: SeverityHigh,
	})

	// BUG-6 FIX: safety filter 错误时采用 fail-closed 原则——默认拒绝，不放行不安全内容
	filtered, filterErr := v.filter.Filter(ctx, content)
	if filterErr != nil {
		result.AddError(ValidationError{
			Code:     ErrCodeContentBlocked,
			Message:  fmt.Sprintf("安全过滤器执行失败 (fail-closed): %s", filterErr.Error()),
			Severity: SeverityCritical,
		})
		// fail-closed: 过滤器出错时返回空的过滤内容，标记为不安全
		result.Metadata["filter_error"] = filterErr.Error()
		result.Metadata["fail_closed"] = true
		return result, nil
	}
	result.Metadata["filtered_content"] = filtered

	return result, nil
}

// AuditEventType 审计事件类型
type AuditEventType string

const (
	// AuditEventValidationFailed 验证失败事件
	AuditEventValidationFailed AuditEventType = "validation_failed"
	// AuditEventContentFiltered 内容过滤事件
	AuditEventContentFiltered AuditEventType = "content_filtered"
	// AuditEventPIIDetected PII 检测事件
	AuditEventPIIDetected AuditEventType = "pii_detected"
	// AuditEventInjectionDetected 注入检测事件
	AuditEventInjectionDetected AuditEventType = "injection_detected"
)

// AuditLogEntry 审计日志条目
// Requirements 2.5: 记录所有验证失败事件用于审计
type AuditLogEntry struct {
	// Timestamp 时间戳
	Timestamp time.Time `json:"timestamp"`
	// EventType 事件类型
	EventType AuditEventType `json:"event_type"`
	// ValidatorName 验证器名称
	ValidatorName string `json:"validator_name"`
	// ContentHash 内容哈希（用于追踪，不存储原始内容）
	ContentHash string `json:"content_hash"`
	// Errors 验证错误列表
	Errors []ValidationError `json:"errors,omitempty"`
	// Metadata 附加元数据
	Metadata map[string]any `json:"metadata,omitempty"`
}

// AuditLogger 护栏层审计日志记录器接口。
//
// 注意：项目中存在三个 AuditLogger 接口，各自服务不同领域，无法统一：
//   - llm.AuditLogger                        — 框架级，记录 AuditEvent（通用事件）
//   - llm/tools.AuditLogger                  — 工具层，记录 *AuditEntry（工具调用/权限/成本），含 LogAsync/Close
//   - agent/guardrails.AuditLogger（本接口） — 护栏层，记录 *AuditLogEntry（验证失败/PII/注入），含 Count
//
// 三者的事件类型、过滤器结构和方法签名均不同，统一会导致接口膨胀。
type AuditLogger interface {
	// Log 记录审计日志
	Log(ctx context.Context, entry *AuditLogEntry) error
	// Query 查询审计日志
	Query(ctx context.Context, filter *AuditLogFilter) ([]*AuditLogEntry, error)
	// Count 统计审计日志数量
	Count(ctx context.Context, filter *AuditLogFilter) (int, error)
}

// AuditLogFilter 审计日志查询过滤器
type AuditLogFilter struct {
	// StartTime 开始时间
	StartTime *time.Time
	// EndTime 结束时间
	EndTime *time.Time
	// EventTypes 事件类型过滤
	EventTypes []AuditEventType
	// ValidatorNames 验证器名称过滤
	ValidatorNames []string
	// Limit 返回数量限制
	Limit int
	// Offset 偏移量
	Offset int
}

// MemoryAuditLogger 内存审计日志记录器
// 用于测试和开发环境
type MemoryAuditLogger struct {
	entries []*AuditLogEntry
	maxSize int
	mu      sync.RWMutex
}

// NewMemoryAuditLogger 创建内存审计日志记录器
func NewMemoryAuditLogger(maxSize int) *MemoryAuditLogger {
	if maxSize <= 0 {
		maxSize = 1000
	}
	return &MemoryAuditLogger{
		entries: make([]*AuditLogEntry, 0),
		maxSize: maxSize,
	}
}

// Log 记录审计日志
func (l *MemoryAuditLogger) Log(ctx context.Context, entry *AuditLogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 如果达到最大容量，移除最旧的条目
	if len(l.entries) >= l.maxSize {
		l.entries = l.entries[1:]
	}

	l.entries = append(l.entries, entry)
	return nil
}

// Query 查询审计日志
func (l *MemoryAuditLogger) Query(ctx context.Context, filter *AuditLogFilter) ([]*AuditLogEntry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var result []*AuditLogEntry

	for _, entry := range l.entries {
		if l.matchFilter(entry, filter) {
			result = append(result, entry)
		}
	}

	// 应用分页
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(result) {
			result = result[filter.Offset:]
		} else if filter.Offset >= len(result) {
			return []*AuditLogEntry{}, nil
		}

		if filter.Limit > 0 && filter.Limit < len(result) {
			result = result[:filter.Limit]
		}
	}

	return result, nil
}

// Count 统计审计日志数量
func (l *MemoryAuditLogger) Count(ctx context.Context, filter *AuditLogFilter) (int, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	count := 0
	for _, entry := range l.entries {
		if l.matchFilter(entry, filter) {
			count++
		}
	}

	return count, nil
}

// GetEntries 获取所有日志条目（用于测试）
func (l *MemoryAuditLogger) GetEntries() []*AuditLogEntry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*AuditLogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

// Clear 清空所有日志条目
func (l *MemoryAuditLogger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make([]*AuditLogEntry, 0)
}

// matchFilter 检查条目是否匹配过滤器
func (l *MemoryAuditLogger) matchFilter(entry *AuditLogEntry, filter *AuditLogFilter) bool {
	if filter == nil {
		return true
	}

	// 时间范围过滤
	if filter.StartTime != nil && entry.Timestamp.Before(*filter.StartTime) {
		return false
	}
	if filter.EndTime != nil && entry.Timestamp.After(*filter.EndTime) {
		return false
	}

	// 事件类型过滤
	if len(filter.EventTypes) > 0 {
		found := false
		for _, et := range filter.EventTypes {
			if entry.EventType == et {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// 验证器名称过滤
	if len(filter.ValidatorNames) > 0 {
		found := false
		for _, name := range filter.ValidatorNames {
			if entry.ValidatorName == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// hashContent 计算内容的 SHA256 哈希
func hashContent(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

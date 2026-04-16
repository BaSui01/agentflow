package guardrails

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ShadowAIConfig配置了阴影AI检测.
type ShadowAIConfig struct {
	EnableNetworkMonitor bool          `json:"enable_network_monitor"`
	EnableContentScan    bool          `json:"enable_content_scan"`
	ScanInterval         time.Duration `json:"scan_interval"`
	AlertThreshold       int           `json:"alert_threshold"`
	WhitelistedDomains   []string      `json:"whitelisted_domains"`
	WhitelistedApps      []string      `json:"whitelisted_apps"`
}

// 默认 ShadowAIConfig 返回默认配置 。
func DefaultShadowAIConfig() ShadowAIConfig {
	return ShadowAIConfig{
		EnableNetworkMonitor: true,
		EnableContentScan:    true,
		ScanInterval:         5 * time.Minute,
		AlertThreshold:       3,
		WhitelistedDomains:   []string{},
		WhitelistedApps:      []string{},
	}
}

// ShadowAI探测器检测出未经授权的AI用法.
type ShadowAIDetector struct {
	config     ShadowAIConfig
	patterns   []*AIPattern
	detections []Detection
	logger     *zap.Logger
	mu         sync.RWMutex
}

// AIPattern是检测AI服务的一种模式.
type AIPattern struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"` // domain, content, api
	Pattern     *regexp.Regexp `json:"-"`
	PatternStr  string         `json:"pattern"`
	Severity    string         `json:"severity"`
	Description string         `json:"description"`
}

// 检测代表检测到的阴影AI用法.
type Detection struct {
	ID          string         `json:"id"`
	PatternName string         `json:"pattern_name"`
	Type        string         `json:"type"`
	Source      string         `json:"source"`
	Evidence    string         `json:"evidence"`
	Severity    string         `json:"severity"`
	Timestamp   time.Time      `json:"timestamp"`
	UserID      string         `json:"user_id,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// NewShadowAI探测器制造出一个新的阴影AI探测器.
func NewShadowAIDetector(config ShadowAIConfig, logger *zap.Logger) *ShadowAIDetector {
	if logger == nil {
		logger = zap.NewNop()
	}

	detector := &ShadowAIDetector{
		config:     config,
		patterns:   getDefaultAIPatterns(),
		detections: make([]Detection, 0),
		logger:     logger.With(zap.String("component", "shadow_ai_detector")),
	}

	return detector
}

// 得到DefaultAIPatters返回默认的AI服务检测模式.
func getDefaultAIPatterns() []*AIPattern {
	patterns := []*AIPattern{
		// 域图案
		{Name: "OpenAI API", Type: "domain", PatternStr: `api\.openai\.com`, Severity: SeverityHigh, Description: "OpenAI API access"},
		{Name: "Claude API", Type: "domain", PatternStr: `api\.anthropic\.com`, Severity: SeverityHigh, Description: "Anthropic Claude API"},
		{Name: "ChatGPT", Type: "domain", PatternStr: `chat\.openai\.com|chatgpt\.com`, Severity: SeverityMedium, Description: "ChatGPT web access"},
		{Name: "Claude Web", Type: "domain", PatternStr: `claude\.ai`, Severity: SeverityMedium, Description: "Claude web access"},
		{Name: "Gemini", Type: "domain", PatternStr: `gemini\.google\.com|generativelanguage\.googleapis\.com`, Severity: SeverityHigh, Description: "Google Gemini"},
		{Name: "Copilot", Type: "domain", PatternStr: `copilot\.microsoft\.com|copilot\.github\.com`, Severity: SeverityMedium, Description: "Microsoft/GitHub Copilot"},
		{Name: "Perplexity", Type: "domain", PatternStr: `perplexity\.ai`, Severity: SeverityLow, Description: "Perplexity AI"},
		{Name: "Hugging Face", Type: "domain", PatternStr: `api\.huggingface\.co|huggingface\.co/api`, Severity: SeverityMedium, Description: "Hugging Face API"},

		// 内容模式( 代码/ 配置中的 API 密钥)
		{Name: "OpenAI Key", Type: "content", PatternStr: `sk-[a-zA-Z0-9]{48}`, Severity: SeverityCritical, Description: "OpenAI API key detected"},
		{Name: "Anthropic Key", Type: "content", PatternStr: `sk-ant-[a-zA-Z0-9-]{95}`, Severity: SeverityCritical, Description: "Anthropic API key detected"},

		// API 请求模式
		{Name: "Chat Completion", Type: "api", PatternStr: `/v1/chat/completions`, Severity: SeverityHigh, Description: "Chat completion API call"},
		{Name: "Embeddings", Type: "api", PatternStr: `/v1/embeddings`, Severity: SeverityMedium, Description: "Embeddings API call"},
	}

	// 编译图案
	for _, p := range patterns {
		p.Pattern = regexp.MustCompile("(?i)" + p.PatternStr)
	}

	return patterns
}

// ScanContent为阴影AI指标扫描内容.
func (d *ShadowAIDetector) ScanContent(ctx context.Context, content, source, userID string) []Detection {
	if !d.config.EnableContentScan {
		return nil
	}

	var detections []Detection

	for _, pattern := range d.patterns {
		if pattern.Type != "content" && pattern.Type != "api" {
			continue
		}

		matches := pattern.Pattern.FindAllString(content, -1)
		for _, match := range matches {
			detection := Detection{
				ID:          generateID(),
				PatternName: pattern.Name,
				Type:        pattern.Type,
				Source:      source,
				Evidence:    maskSensitive(match),
				Severity:    pattern.Severity,
				Timestamp:   time.Now(),
				UserID:      userID,
			}
			detections = append(detections, detection)
		}
	}

	d.recordDetections(detections)
	return detections
}

// 检查域名检查是否为AI服务.
func (d *ShadowAIDetector) CheckDomain(domain string) *Detection {
	// 检查白名单
	for _, w := range d.config.WhitelistedDomains {
		if domain == w {
			return nil
		}
	}

	for _, pattern := range d.patterns {
		if pattern.Type != "domain" {
			continue
		}

		if pattern.Pattern.MatchString(domain) {
			detection := Detection{
				ID:          generateID(),
				PatternName: pattern.Name,
				Type:        "domain",
				Source:      domain,
				Evidence:    domain,
				Severity:    pattern.Severity,
				Timestamp:   time.Now(),
			}
			d.recordDetections([]Detection{detection})
			return &detection
		}
	}

	return nil
}

func (d *ShadowAIDetector) recordDetections(detections []Detection) {
	if len(detections) == 0 {
		return
	}

	d.mu.Lock()
	d.detections = append(d.detections, detections...)
	// 保留最后的一万次检测
	if len(d.detections) > 10000 {
		d.detections = d.detections[len(d.detections)-10000:]
	}
	d.mu.Unlock()

	for _, det := range detections {
		d.logger.Warn("shadow AI detected",
			zap.String("pattern", det.PatternName),
			zap.String("severity", det.Severity),
			zap.String("source", det.Source),
		)
	}
}

// Get Detections 返回最近的检测。
func (d *ShadowAIDetector) GetDetections(limit int) []Detection {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 || limit > len(d.detections) {
		limit = len(d.detections)
	}

	start := len(d.detections) - limit
	return append([]Detection{}, d.detections[start:]...)
}

// GetStats 返回检测统计.
func (d *ShadowAIDetector) GetStats() ShadowAIStats {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := ShadowAIStats{
		TotalDetections: len(d.detections),
		BySeverity:      make(map[string]int),
		ByPattern:       make(map[string]int),
	}

	for _, det := range d.detections {
		stats.BySeverity[det.Severity]++
		stats.ByPattern[det.PatternName]++
	}

	return stats
}

// ShadowAIStats包含检测统计数据.
type ShadowAIStats struct {
	TotalDetections int            `json:"total_detections"`
	BySeverity      map[string]int `json:"by_severity"`
	ByPattern       map[string]int `json:"by_pattern"`
}

// 添加Pattern 添加自定义检测模式 。
func (d *ShadowAIDetector) AddPattern(name, patternType, patternStr, severity, description string) error {
	re, err := regexp.Compile("(?i)" + patternStr)
	if err != nil {
		return err
	}

	d.mu.Lock()
	d.patterns = append(d.patterns, &AIPattern{
		Name:        name,
		Type:        patternType,
		Pattern:     re,
		PatternStr:  patternStr,
		Severity:    severity,
		Description: description,
	})
	d.mu.Unlock()

	return nil
}

func generateID() string {
	return fmt.Sprintf("det_%d", time.Now().UnixNano())
}

func maskSensitive(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

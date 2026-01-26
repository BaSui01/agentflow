// Package guardrails provides Shadow AI detection for enterprise environments.
// Detects unauthorized AI usage within organizations.
package guardrails

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ShadowAIConfig configures shadow AI detection.
type ShadowAIConfig struct {
	EnableNetworkMonitor bool          `json:"enable_network_monitor"`
	EnableContentScan    bool          `json:"enable_content_scan"`
	ScanInterval         time.Duration `json:"scan_interval"`
	AlertThreshold       int           `json:"alert_threshold"`
	WhitelistedDomains   []string      `json:"whitelisted_domains"`
	WhitelistedApps      []string      `json:"whitelisted_apps"`
}

// DefaultShadowAIConfig returns default configuration.
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

// ShadowAIDetector detects unauthorized AI usage.
type ShadowAIDetector struct {
	config     ShadowAIConfig
	patterns   []*AIPattern
	detections []Detection
	logger     *zap.Logger
	mu         sync.RWMutex
}

// AIPattern represents a pattern for detecting AI services.
type AIPattern struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"` // domain, content, api
	Pattern     *regexp.Regexp `json:"-"`
	PatternStr  string         `json:"pattern"`
	Severity    string         `json:"severity"`
	Description string         `json:"description"`
}

// Detection represents a detected shadow AI usage.
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

// NewShadowAIDetector creates a new shadow AI detector.
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

// getDefaultAIPatterns returns default AI service detection patterns.
func getDefaultAIPatterns() []*AIPattern {
	patterns := []*AIPattern{
		// Domain patterns
		{Name: "OpenAI API", Type: "domain", PatternStr: `api\.openai\.com`, Severity: SeverityHigh, Description: "OpenAI API access"},
		{Name: "Claude API", Type: "domain", PatternStr: `api\.anthropic\.com`, Severity: SeverityHigh, Description: "Anthropic Claude API"},
		{Name: "ChatGPT", Type: "domain", PatternStr: `chat\.openai\.com|chatgpt\.com`, Severity: SeverityMedium, Description: "ChatGPT web access"},
		{Name: "Claude Web", Type: "domain", PatternStr: `claude\.ai`, Severity: SeverityMedium, Description: "Claude web access"},
		{Name: "Gemini", Type: "domain", PatternStr: `gemini\.google\.com|generativelanguage\.googleapis\.com`, Severity: SeverityHigh, Description: "Google Gemini"},
		{Name: "Copilot", Type: "domain", PatternStr: `copilot\.microsoft\.com|copilot\.github\.com`, Severity: SeverityMedium, Description: "Microsoft/GitHub Copilot"},
		{Name: "Perplexity", Type: "domain", PatternStr: `perplexity\.ai`, Severity: SeverityLow, Description: "Perplexity AI"},
		{Name: "Hugging Face", Type: "domain", PatternStr: `api\.huggingface\.co|huggingface\.co/api`, Severity: SeverityMedium, Description: "Hugging Face API"},

		// Content patterns (API keys in code/config)
		{Name: "OpenAI Key", Type: "content", PatternStr: `sk-[a-zA-Z0-9]{48}`, Severity: SeverityCritical, Description: "OpenAI API key detected"},
		{Name: "Anthropic Key", Type: "content", PatternStr: `sk-ant-[a-zA-Z0-9-]{95}`, Severity: SeverityCritical, Description: "Anthropic API key detected"},

		// API request patterns
		{Name: "Chat Completion", Type: "api", PatternStr: `/v1/chat/completions`, Severity: SeverityHigh, Description: "Chat completion API call"},
		{Name: "Embeddings", Type: "api", PatternStr: `/v1/embeddings`, Severity: SeverityMedium, Description: "Embeddings API call"},
	}

	// Compile patterns
	for _, p := range patterns {
		p.Pattern = regexp.MustCompile("(?i)" + p.PatternStr)
	}

	return patterns
}

// ScanContent scans content for shadow AI indicators.
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

// CheckDomain checks if a domain is an AI service.
func (d *ShadowAIDetector) CheckDomain(domain string) *Detection {
	// Check whitelist
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
	// Keep last 10000 detections
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

// GetDetections returns recent detections.
func (d *ShadowAIDetector) GetDetections(limit int) []Detection {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if limit <= 0 || limit > len(d.detections) {
		limit = len(d.detections)
	}

	start := len(d.detections) - limit
	return append([]Detection{}, d.detections[start:]...)
}

// GetStats returns detection statistics.
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

// ShadowAIStats contains detection statistics.
type ShadowAIStats struct {
	TotalDetections int            `json:"total_detections"`
	BySeverity      map[string]int `json:"by_severity"`
	ByPattern       map[string]int `json:"by_pattern"`
}

// AddPattern adds a custom detection pattern.
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

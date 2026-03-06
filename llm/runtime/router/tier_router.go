package router

import (
	"strings"

	"go.uber.org/zap"
)

// ModelTier represents a complexity-based model tier.
type ModelTier string

const (
	TierNano     ModelTier = "nano"
	TierStandard ModelTier = "standard"
	TierFrontier ModelTier = "frontier"
)

// TierConfig maps tiers to model names.
type TierConfig struct {
	Enabled           bool     `json:"enabled" yaml:"enabled"`
	NanoModels        []string `json:"nano_models" yaml:"nano_models"`
	StandardModels    []string `json:"standard_models" yaml:"standard_models"`
	FrontierModels    []string `json:"frontier_models" yaml:"frontier_models"`
	NanoThreshold     int      `json:"nano_threshold" yaml:"nano_threshold"`
	FrontierThreshold int      `json:"frontier_threshold" yaml:"frontier_threshold"`
}

// DefaultTierConfig returns sensible defaults.
func DefaultTierConfig() TierConfig {
	return TierConfig{
		Enabled:           false,
		NanoModels:        []string{"gpt-4o-mini", "claude-haiku", "gemini-flash"},
		StandardModels:    []string{"gpt-4o", "claude-sonnet", "gemini-pro"},
		FrontierModels:    []string{"gpt-4.5", "claude-opus", "gemini-ultra"},
		NanoThreshold:     30,
		FrontierThreshold: 70,
	}
}

// TierRouter scores request complexity and selects model tier.
type TierRouter struct {
	config TierConfig
	logger *zap.Logger
}

// NewTierRouter creates a tier router.
func NewTierRouter(config TierConfig, logger *zap.Logger) *TierRouter {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.NanoThreshold <= 0 {
		config.NanoThreshold = 30
	}
	if config.FrontierThreshold <= 0 {
		config.FrontierThreshold = 70
	}
	return &TierRouter{config: config, logger: logger}
}

// ScoreComplexity returns 0-100 complexity score for a chat request.
func (t *TierRouter) ScoreComplexity(req *ChatRequest) int {
	if req == nil {
		return 50
	}

	score := 0

	// Factor 1: Total message count (0-25 points)
	msgCount := len(req.Messages)
	switch {
	case msgCount <= 2:
		score += 5
	case msgCount <= 5:
		score += 10
	case msgCount <= 10:
		score += 15
	case msgCount <= 20:
		score += 20
	default:
		score += 25
	}

	// Factor 2: Total content length (0-25 points)
	totalLen := 0
	for _, m := range req.Messages {
		totalLen += len(m.Content)
	}
	switch {
	case totalLen < 500:
		score += 5
	case totalLen < 2000:
		score += 10
	case totalLen < 5000:
		score += 15
	case totalLen < 10000:
		score += 20
	default:
		score += 25
	}

	// Factor 3: Tools present (0-25 points)
	toolCount := len(req.Tools)
	switch {
	case toolCount == 0:
		score += 0
	case toolCount <= 3:
		score += 10
	case toolCount <= 8:
		score += 15
	default:
		score += 25
	}

	// Factor 4: System prompt complexity (0-25 points)
	for _, m := range req.Messages {
		if m.Role == "system" {
			sysLen := len(m.Content)
			switch {
			case sysLen < 200:
				score += 5
			case sysLen < 1000:
				score += 10
			case sysLen < 3000:
				score += 15
			default:
				score += 25
			}
			break
		}
	}

	return score
}

// SelectTier maps a complexity score to a model tier.
func (t *TierRouter) SelectTier(score int) ModelTier {
	switch {
	case score <= t.config.NanoThreshold:
		return TierNano
	case score >= t.config.FrontierThreshold:
		return TierFrontier
	default:
		return TierStandard
	}
}

// SelectModel picks the first available model for a tier, preferring models
// that match the family of the originally requested model.
func (t *TierRouter) SelectModel(tier ModelTier, originalModel string) string {
	var candidates []string
	switch tier {
	case TierNano:
		candidates = t.config.NanoModels
	case TierStandard:
		candidates = t.config.StandardModels
	case TierFrontier:
		candidates = t.config.FrontierModels
	}
	if len(candidates) == 0 {
		return originalModel
	}

	family := extractFamily(originalModel)
	if family != "" {
		for _, c := range candidates {
			if strings.Contains(strings.ToLower(c), family) {
				return c
			}
		}
	}
	return candidates[0]
}

// ResolveModel applies tier routing to select the optimal model.
// Returns the original model if tier routing is disabled.
func (t *TierRouter) ResolveModel(req *ChatRequest) string {
	if req == nil {
		return ""
	}
	if !t.config.Enabled {
		return req.Model
	}
	score := t.ScoreComplexity(req)
	tier := t.SelectTier(score)
	model := t.SelectModel(tier, req.Model)
	t.logger.Debug("tier routing resolved",
		zap.Int("complexity_score", score),
		zap.String("tier", string(tier)),
		zap.String("original_model", req.Model),
		zap.String("resolved_model", model))
	return model
}

func extractFamily(model string) string {
	lower := strings.ToLower(model)
	families := []string{"gpt", "claude", "gemini", "deepseek", "qwen", "glm", "grok", "kimi", "mistral", "minimax", "llama"}
	for _, f := range families {
		if strings.Contains(lower, f) {
			return f
		}
	}
	return ""
}

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

// ScoringWeights controls how much each complexity factor contributes.
// Default weight for each factor is 25, giving a total max score of 100.
// Adjust individual weights to bias the routing toward specific factors.
type ScoringWeights struct {
	MessageCount  int `json:"message_count" yaml:"message_count"`
	ContentLength int `json:"content_length" yaml:"content_length"`
	ToolCount     int `json:"tool_count" yaml:"tool_count"`
	SystemPrompt  int `json:"system_prompt" yaml:"system_prompt"`
}

// DefaultScoringWeights returns equal weights totaling 100.
func DefaultScoringWeights() ScoringWeights {
	return ScoringWeights{
		MessageCount:  25,
		ContentLength: 25,
		ToolCount:     25,
		SystemPrompt:  25,
	}
}

// TierConfig maps tiers to model names.
type TierConfig struct {
	Enabled           bool           `json:"enabled" yaml:"enabled"`
	NanoModels        []string       `json:"nano_models" yaml:"nano_models"`
	StandardModels    []string       `json:"standard_models" yaml:"standard_models"`
	FrontierModels    []string       `json:"frontier_models" yaml:"frontier_models"`
	NanoThreshold     int            `json:"nano_threshold" yaml:"nano_threshold"`
	FrontierThreshold int            `json:"frontier_threshold" yaml:"frontier_threshold"`
	Weights           ScoringWeights `json:"weights" yaml:"weights"`
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
		Weights:           DefaultScoringWeights(),
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
	w := config.Weights
	if w.MessageCount == 0 && w.ContentLength == 0 && w.ToolCount == 0 && w.SystemPrompt == 0 {
		config.Weights = DefaultScoringWeights()
	}
	return &TierRouter{config: config, logger: logger}
}

// ScoreComplexity returns a complexity score for a chat request.
// Each factor produces a raw 0-25 value, then scales by its configured weight.
// With default weights (all 25), the score range is 0-100.
func (t *TierRouter) ScoreComplexity(req *ChatRequest) int {
	if req == nil {
		return 50
	}

	w := t.config.Weights
	score := 0

	score += applyWeight(msgScore(len(req.Messages)), w.MessageCount)
	score += applyWeight(contentLenScore(req.Messages), w.ContentLength)
	score += applyWeight(toolScore(len(req.Tools)), w.ToolCount)
	score += applyWeight(systemPromptScore(req.Messages), w.SystemPrompt)

	return score
}

func msgScore(count int) int {
	switch {
	case count <= 2:
		return 5
	case count <= 5:
		return 10
	case count <= 10:
		return 15
	case count <= 20:
		return 20
	default:
		return 25
	}
}

func contentLenScore(msgs []Message) int {
	totalLen := 0
	for _, m := range msgs {
		totalLen += len(m.Content)
	}
	switch {
	case totalLen < 500:
		return 5
	case totalLen < 2000:
		return 10
	case totalLen < 5000:
		return 15
	case totalLen < 10000:
		return 20
	default:
		return 25
	}
}

func toolScore(count int) int {
	switch {
	case count == 0:
		return 0
	case count <= 3:
		return 10
	case count <= 8:
		return 15
	default:
		return 25
	}
}

func systemPromptScore(msgs []Message) int {
	for _, m := range msgs {
		if m.Role == "system" {
			sysLen := len(m.Content)
			switch {
			case sysLen < 200:
				return 5
			case sysLen < 1000:
				return 10
			case sysLen < 3000:
				return 15
			default:
				return 25
			}
		}
	}
	return 0
}

// applyWeight scales a raw factor score (0-25) by a configured weight.
// With default weight 25, the output equals the raw score.
func applyWeight(rawScore, weight int) int {
	return rawScore * weight / 25
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

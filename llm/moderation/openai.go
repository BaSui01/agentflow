package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// OpenAIProvider 使用 OpenAI API 执行内容审核.
type OpenAIProvider struct {
	*providers.BaseCapabilityProvider
}

// NewOpenAIProvider 创建新的 OpenAI 内容审核提供者.
func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "omni-moderation-latest"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &OpenAIProvider{
		BaseCapabilityProvider: providers.NewBaseCapabilityProvider(providers.CapabilityConfig{
			Name:    "openai-moderation",
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
			Timeout: timeout,
		}),
	}
}

func (p *OpenAIProvider) Name() string { return p.ProviderName }

type openAIModerationRequest struct {
	Model string `json:"model,omitempty"`
	Input any    `json:"input"`
}

type openAIModerationResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Results []struct {
		Flagged        bool               `json:"flagged"`
		Categories     map[string]bool    `json:"categories"`
		CategoryScores map[string]float64 `json:"category_scores"`
	} `json:"results"`
}

func (p *OpenAIProvider) Moderate(ctx context.Context, req *ModerationRequest) (*ModerationResponse, error) {
	if req == nil || len(req.Input) == 0 {
		return nil, fmt.Errorf("input is required")
	}

	input := make([]any, 0, len(req.Input)+len(req.Images))
	for _, text := range req.Input {
		input = append(input, text)
	}
	for _, img := range req.Images {
		input = append(input, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:image/png;base64," + img,
			},
		})
	}

	model := providers.ChooseCapabilityModel(req.Model, p.Model)
	payload := openAIModerationRequest{
		Model: model,
		Input: input,
	}

	var resp openAIModerationResponse
	if err := p.PostJSONDecode(ctx, "/moderations", payload, &resp); err != nil {
		return nil, fmt.Errorf("openai moderation error: %w", err)
	}

	out := &ModerationResponse{
		Provider:  p.Name(),
		Model:     providers.ChooseCapabilityModel(resp.Model, model),
		Results:   make([]ModerationResult, 0, len(resp.Results)),
		CreatedAt: time.Now(),
	}
	for _, r := range resp.Results {
		out.Results = append(out.Results, ModerationResult{
			Flagged:    r.Flagged,
			Categories: mapCategories(r.Categories),
			Scores:     mapScores(r.CategoryScores),
		})
	}
	return out, nil
}

func mapCategories(cats map[string]bool) ModerationCategory {
	return ModerationCategory{
		Hate:            cats["hate"],
		HateThreatening: cats["hate/threatening"],
		Harassment:      cats["harassment"],
		SelfHarm:        cats["self-harm"],
		SelfHarmIntent:  cats["self-harm/intent"],
		Sexual:          cats["sexual"],
		SexualMinors:    cats["sexual/minors"],
		Violence:        cats["violence"],
		ViolenceGraphic: cats["violence/graphic"],
		Illicit:         cats["illicit"],
		IllicitViolent:  cats["illicit/violent"],
	}
}

func mapScores(scores map[string]float64) ModerationScores {
	return ModerationScores{
		Hate:            scores["hate"],
		HateThreatening: scores["hate/threatening"],
		Harassment:      scores["harassment"],
		SelfHarm:        scores["self-harm"],
		SelfHarmIntent:  scores["self-harm/intent"],
		Sexual:          scores["sexual"],
		SexualMinors:    scores["sexual/minors"],
		Violence:        scores["violence"],
		ViolenceGraphic: scores["violence/graphic"],
		Illicit:         scores["illicit"],
		IllicitViolent:  scores["illicit/violent"],
	}
}

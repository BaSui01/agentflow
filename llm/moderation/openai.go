package moderation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIProvider使用OpenAI API执行节制.
type OpenAIProvider struct {
	cfg    OpenAIConfig
	client *http.Client
}

// NewOpenAIProvider创建了一个新的OpenAI温和提供商.
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
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *OpenAIProvider) Name() string { return "openai-moderation" }

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

// 适度检查政策违规内容.
func (p *OpenAIProvider) Moderate(ctx context.Context, req *ModerationRequest) (*ModerationResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	// 构建输入 - 仅文本或多模式
	var input any
	if len(req.Images) > 0 {
		// 多式联运投入
		var items []map[string]any
		for _, text := range req.Input {
			items = append(items, map[string]any{"type": "text", "text": text})
		}
		for _, img := range req.Images {
			items = append(items, map[string]any{
				"type":      "image_url",
				"image_url": map[string]string{"url": "data:image/jpeg;base64," + img},
			})
		}
		input = items
	} else {
		input = req.Input
	}

	body := openAIModerationRequest{Model: model, Input: input}
	payload, _ := json.Marshal(body)

	endpoint := fmt.Sprintf("%s/moderations", strings.TrimRight(p.cfg.BaseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("moderation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("moderation error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var oResp openAIModerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&oResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	results := make([]ModerationResult, len(oResp.Results))
	for i, r := range oResp.Results {
		results[i] = ModerationResult{
			Flagged:    r.Flagged,
			Categories: mapCategories(r.Categories),
			Scores:     mapScores(r.CategoryScores),
		}
	}

	return &ModerationResponse{
		Provider:  p.Name(),
		Model:     oResp.Model,
		Results:   results,
		CreatedAt: time.Now(),
	}, nil
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

package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// LLMVisionProvider LLM 视觉能力提供者接口
type LLMVisionProvider interface {
	// AnalyzeImage 分析图片，返回 JSON 格式的分析结果
	AnalyzeImage(ctx context.Context, imageBase64 string, prompt string) (string, error)
}

// LLMVisionAdapter 将 LLM 视觉能力适配为 VisionModel 接口
type LLMVisionAdapter struct {
	provider LLMVisionProvider
	logger   *zap.Logger
}

// NewLLMVisionAdapter 创建视觉适配器
func NewLLMVisionAdapter(provider LLMVisionProvider, logger *zap.Logger) *LLMVisionAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LLMVisionAdapter{
		provider: provider,
		logger:   logger.With(zap.String("component", "vision_adapter")),
	}
}

// Analyze 分析截图
func (a *LLMVisionAdapter) Analyze(ctx context.Context, screenshot *Screenshot) (*VisionAnalysis, error) {
	if screenshot == nil || len(screenshot.Data) == 0 {
		return nil, fmt.Errorf("empty screenshot")
	}

	imageB64 := base64.StdEncoding.EncodeToString(screenshot.Data)

	prompt := `Analyze this webpage screenshot and return a JSON object with:
{
  "elements": [{"id": "elem_N", "type": "button|input|link|text|image", "text": "visible text", "x": 0, "y": 0, "width": 0, "height": 0, "clickable": true, "confidence": 0.9}],
  "page_title": "page title",
  "page_type": "login|search|form|article|dashboard|other",
  "description": "brief description of what's on the page",
  "suggestions": ["possible actions"]
}
Only return valid JSON, no markdown.`

	response, err := a.provider.AnalyzeImage(ctx, imageB64, prompt)
	if err != nil {
		return nil, fmt.Errorf("vision analysis failed: %w", err)
	}

	var analysis VisionAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		a.logger.Warn("failed to parse vision response as JSON, using raw",
			zap.Error(err))
		analysis = VisionAnalysis{
			Description: response,
			PageTitle:   screenshot.URL,
		}
	}

	a.logger.Debug("vision analysis complete",
		zap.Int("elements", len(analysis.Elements)),
		zap.String("page_type", analysis.PageType))

	return &analysis, nil
}

// PlanActions 规划下一步操作
func (a *LLMVisionAdapter) PlanActions(ctx context.Context, goal string, analysis *VisionAnalysis) ([]AgenticAction, error) {
	analysisJSON, err := json.Marshal(analysis)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal analysis: %w", err)
	}

	prompt := fmt.Sprintf(`Given the goal: "%s"
And the current page analysis: %s

Plan the next browser action. Return a JSON array of actions:
[{"type": "click|type|scroll|navigate|wait", "selector": "css selector if applicable", "value": "text to type or url", "x": 0, "y": 0}]

Rules:
- Return at most 3 actions
- Prefer clicking interactive elements to achieve the goal
- If a form needs filling, use "type" action
- Only return valid JSON array, no markdown.`, goal, string(analysisJSON))

	response, err := a.provider.AnalyzeImage(ctx, "", prompt)
	if err != nil {
		return nil, fmt.Errorf("action planning failed: %w", err)
	}

	var actions []AgenticAction
	if err := json.Unmarshal([]byte(response), &actions); err != nil {
		a.logger.Warn("failed to parse action plan", zap.Error(err))
		return nil, fmt.Errorf("failed to parse action plan: %w", err)
	}

	return actions, nil
}

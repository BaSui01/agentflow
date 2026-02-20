// 一揽子节制提供了内容节制能力。
package moderation

import (
	"context"
	"time"
)

// 调和 Provider 为内容节制定义了接口.
type ModerationProvider interface {
	Name() string
	Moderate(ctx context.Context, req *ModerationRequest) (*ModerationResponse, error)
}

// 温和请求是温和请求。
type ModerationRequest struct {
	Input  []string `json:"input"`            // Text inputs to moderate
	Images []string `json:"images,omitempty"` // Base64 images (optional)
	Model  string   `json:"model,omitempty"`  // Model to use
}

// 温和反应是一种温和的反应。
type ModerationResponse struct {
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	Results   []ModerationResult `json:"results"`
	CreatedAt time.Time          `json:"created_at"`
}

// 中度Result代表单一输入的结果.
type ModerationResult struct {
	Flagged    bool               `json:"flagged"`
	Categories ModerationCategory `json:"categories"`
	Scores     ModerationScores   `json:"scores"`
}

// 中度类别表示标出哪些类别 。
type ModerationCategory struct {
	Hate            bool `json:"hate"`
	HateThreatening bool `json:"hate_threatening"`
	Harassment      bool `json:"harassment"`
	SelfHarm        bool `json:"self_harm"`
	SelfHarmIntent  bool `json:"self_harm_intent"`
	Sexual          bool `json:"sexual"`
	SexualMinors    bool `json:"sexual_minors"`
	Violence        bool `json:"violence"`
	ViolenceGraphic bool `json:"violence_graphic"`
	Illicit         bool `json:"illicit"`
	IllicitViolent  bool `json:"illicit_violent"`
}

// 中调分数包含每个类别的信心分数.
type ModerationScores struct {
	Hate            float64 `json:"hate"`
	HateThreatening float64 `json:"hate_threatening"`
	Harassment      float64 `json:"harassment"`
	SelfHarm        float64 `json:"self_harm"`
	SelfHarmIntent  float64 `json:"self_harm_intent"`
	Sexual          float64 `json:"sexual"`
	SexualMinors    float64 `json:"sexual_minors"`
	Violence        float64 `json:"violence"`
	ViolenceGraphic float64 `json:"violence_graphic"`
	Illicit         float64 `json:"illicit"`
	IllicitViolent  float64 `json:"illicit_violent"`
}

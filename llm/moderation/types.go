// Package moderation provides content moderation capabilities.
package moderation

import (
	"context"
	"time"
)

// ModerationProvider defines the interface for content moderation.
type ModerationProvider interface {
	Name() string
	Moderate(ctx context.Context, req *ModerationRequest) (*ModerationResponse, error)
}

// ModerationRequest represents a moderation request.
type ModerationRequest struct {
	Input  []string `json:"input"`            // Text inputs to moderate
	Images []string `json:"images,omitempty"` // Base64 images (optional)
	Model  string   `json:"model,omitempty"`  // Model to use
}

// ModerationResponse represents a moderation response.
type ModerationResponse struct {
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	Results   []ModerationResult `json:"results"`
	CreatedAt time.Time          `json:"created_at"`
}

// ModerationResult represents the result for a single input.
type ModerationResult struct {
	Flagged    bool               `json:"flagged"`
	Categories ModerationCategory `json:"categories"`
	Scores     ModerationScores   `json:"scores"`
}

// ModerationCategory indicates which categories were flagged.
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

// ModerationScores contains confidence scores for each category.
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

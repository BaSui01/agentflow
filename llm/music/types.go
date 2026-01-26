// Package music provides AI music generation capabilities.
package music

import (
	"context"
	"time"
)

// MusicProvider defines the interface for music generation.
type MusicProvider interface {
	Name() string
	Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)
}

// GenerateRequest represents a music generation request.
type GenerateRequest struct {
	Prompt         string  `json:"prompt"`                    // Text description or lyrics
	Style          string  `json:"style,omitempty"`           // Music style (pop, rock, jazz, etc.)
	Duration       float64 `json:"duration,omitempty"`        // Duration in seconds
	Instrumental   bool    `json:"instrumental,omitempty"`    // Generate without vocals
	Model          string  `json:"model,omitempty"`           // Model to use
	ContinueFrom   string  `json:"continue_from,omitempty"`   // Audio clip ID to extend
	ReferenceAudio string  `json:"reference_audio,omitempty"` // Base64 reference audio
}

// GenerateResponse represents a music generation response.
type GenerateResponse struct {
	Provider  string      `json:"provider"`
	Model     string      `json:"model"`
	Tracks    []MusicData `json:"tracks"`
	Usage     MusicUsage  `json:"usage"`
	CreatedAt time.Time   `json:"created_at"`
}

// MusicData represents a generated music track.
type MusicData struct {
	ID       string  `json:"id,omitempty"`
	URL      string  `json:"url,omitempty"`       // Download URL
	B64Audio string  `json:"b64_audio,omitempty"` // Base64 encoded audio
	Duration float64 `json:"duration"`            // Duration in seconds
	Title    string  `json:"title,omitempty"`
	Lyrics   string  `json:"lyrics,omitempty"`
	Style    string  `json:"style,omitempty"`
}

// MusicUsage contains usage information.
type MusicUsage struct {
	TracksGenerated int     `json:"tracks_generated"`
	DurationSeconds float64 `json:"duration_seconds"`
	Credits         float64 `json:"credits,omitempty"`
}

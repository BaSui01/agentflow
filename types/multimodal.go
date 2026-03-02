package types

// VideoGenerationMode defines unified generation mode for video capabilities.
type VideoGenerationMode string

const (
	VideoModeTextToVideo  VideoGenerationMode = "text_to_video"
	VideoModeImageToVideo VideoGenerationMode = "image_to_video"
)

// IsValid returns whether this video generation mode is supported.
func (m VideoGenerationMode) IsValid() bool {
	switch m {
	case VideoModeTextToVideo, VideoModeImageToVideo:
		return true
	default:
		return false
	}
}

// NarrationProfile defines a unified speech profile for TTS/narration/avatar driving.
type NarrationProfile struct {
	Voice    string  `json:"voice,omitempty"`
	Style    string  `json:"style,omitempty"`
	Emotion  string  `json:"emotion,omitempty"`
	Language string  `json:"language,omitempty"`
	Rate     float64 `json:"rate,omitempty"`
	Pitch    float64 `json:"pitch,omitempty"`
	Volume   float64 `json:"volume,omitempty"`
	PauseMS  int     `json:"pause_ms,omitempty"`
}

// AvatarDriveMode defines unified drive mode for avatar generation.
type AvatarDriveMode string

const (
	AvatarDriveModeText  AvatarDriveMode = "text"
	AvatarDriveModeAudio AvatarDriveMode = "audio"
	AvatarDriveModeVideo AvatarDriveMode = "video"
)

// IsValid returns whether this avatar drive mode is supported.
func (m AvatarDriveMode) IsValid() bool {
	switch m {
	case AvatarDriveModeText, AvatarDriveModeAudio, AvatarDriveModeVideo:
		return true
	default:
		return false
	}
}

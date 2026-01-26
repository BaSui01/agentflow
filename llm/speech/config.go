package speech

import "time"

// OpenAITTSConfig configures OpenAI TTS provider.
type OpenAITTSConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // tts-1, tts-1-hd
	Voice   string        `json:"voice,omitempty" yaml:"voice,omitempty"` // alloy, echo, fable, onyx, nova, shimmer
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// OpenAISTTConfig configures OpenAI Whisper STT provider.
type OpenAISTTConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // whisper-1
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// ElevenLabsConfig configures ElevenLabs TTS provider.
type ElevenLabsConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // eleven_multilingual_v2
	VoiceID string        `json:"voice_id,omitempty" yaml:"voice_id,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DeepgramConfig configures Deepgram STT provider.
type DeepgramConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // nova-2
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DefaultOpenAITTSConfig returns default OpenAI TTS config.
func DefaultOpenAITTSConfig() OpenAITTSConfig {
	return OpenAITTSConfig{
		BaseURL: "https://api.openai.com",
		Model:   "tts-1-hd",
		Voice:   "alloy",
		Timeout: 60 * time.Second,
	}
}

// DefaultOpenAISTTConfig returns default OpenAI STT config.
func DefaultOpenAISTTConfig() OpenAISTTConfig {
	return OpenAISTTConfig{
		BaseURL: "https://api.openai.com",
		Model:   "whisper-1",
		Timeout: 120 * time.Second,
	}
}

// DefaultElevenLabsConfig returns default ElevenLabs config.
func DefaultElevenLabsConfig() ElevenLabsConfig {
	return ElevenLabsConfig{
		BaseURL: "https://api.elevenlabs.io",
		Model:   "eleven_multilingual_v2",
		Timeout: 60 * time.Second,
	}
}

// DefaultDeepgramConfig returns default Deepgram config.
func DefaultDeepgramConfig() DeepgramConfig {
	return DeepgramConfig{
		BaseURL: "https://api.deepgram.com",
		Model:   "nova-2",
		Timeout: 120 * time.Second,
	}
}

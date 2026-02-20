package speech

import "time"

// OpenAITTSConfig 配置 OpenAI TTS 提供者.
type OpenAITTSConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // tts-1, tts-1-hd
	Voice   string        `json:"voice,omitempty" yaml:"voice,omitempty"` // alloy, echo, fable, onyx, nova, shimmer
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// OpenAISTTConfig 配置 OpenAI Whisper STT 提供者.
type OpenAISTTConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // whisper-1
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// ElevenLabsConfig 配置 ElevenLabs TTS 提供者.
type ElevenLabsConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // eleven_multilingual_v2
	VoiceID string        `json:"voice_id,omitempty" yaml:"voice_id,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// DeepgramConfig 配置 Deepgram STT 提供者.
type DeepgramConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"` // nova-2
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// 默认 OpenAITTSConfig 返回默认 OpenAI TTS 配置 。
func DefaultOpenAITTSConfig() OpenAITTSConfig {
	return OpenAITTSConfig{
		BaseURL: "https://api.openai.com",
		Model:   "tts-1-hd",
		Voice:   "alloy",
		Timeout: 60 * time.Second,
	}
}

// 默认 OpenAISTTConfig 返回默认 OpenAI STT 配置 。
func DefaultOpenAISTTConfig() OpenAISTTConfig {
	return OpenAISTTConfig{
		BaseURL: "https://api.openai.com",
		Model:   "whisper-1",
		Timeout: 120 * time.Second,
	}
}

// 默认ElevenLabsconfig 返回默认的 11Labs 配置 。
func DefaultElevenLabsConfig() ElevenLabsConfig {
	return ElevenLabsConfig{
		BaseURL: "https://api.elevenlabs.io",
		Model:   "eleven_multilingual_v2",
		Timeout: 60 * time.Second,
	}
}

// 默认 DepgramConfig 返回默认 Depgram 配置 。
func DefaultDeepgramConfig() DeepgramConfig {
	return DeepgramConfig{
		BaseURL: "https://api.deepgram.com",
		Model:   "nova-2",
		Timeout: 120 * time.Second,
	}
}

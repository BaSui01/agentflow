package speech

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// OpenAITTSConfig 配置 OpenAI TTS 提供者.
type OpenAITTSConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
	Voice                        string `json:"voice,omitempty" yaml:"voice,omitempty"` // alloy, echo, fable, onyx, nova, shimmer
}

// OpenAISTTConfig 配置 OpenAI Whisper STT 提供者.
type OpenAISTTConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// ElevenLabsConfig 配置 ElevenLabs TTS 提供者.
type ElevenLabsConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
	VoiceID                      string `json:"voice_id,omitempty" yaml:"voice_id,omitempty"`
}

// DeepgramConfig 配置 Deepgram STT 提供者.
type DeepgramConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultOpenAITTSConfig 返回默认 OpenAI TTS 配置.
func DefaultOpenAITTSConfig() OpenAITTSConfig {
	return OpenAITTSConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com",
			Model:   "tts-1-hd",
			Timeout: 60 * time.Second,
		},
		Voice: "alloy",
	}
}

// DefaultOpenAISTTConfig 返回默认 OpenAI STT 配置.
func DefaultOpenAISTTConfig() OpenAISTTConfig {
	return OpenAISTTConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com",
			Model:   "whisper-1",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultElevenLabsConfig 返回默认 ElevenLabs 配置.
func DefaultElevenLabsConfig() ElevenLabsConfig {
	return ElevenLabsConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.elevenlabs.io",
			Model:   "eleven_multilingual_v2",
			Timeout: 60 * time.Second,
		},
	}
}

// DefaultDeepgramConfig 返回默认 Deepgram 配置.
func DefaultDeepgramConfig() DeepgramConfig {
	return DeepgramConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.deepgram.com",
			Model:   "nova-2",
			Timeout: 120 * time.Second,
		},
	}
}


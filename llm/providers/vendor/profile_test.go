package vendor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewOpenAIProfile_AggregatesCapabilities(t *testing.T) {
	profile := NewOpenAIProfile(OpenAIConfig{
		APIKey:         "sk-openai",
		BaseURL:        "https://api.openai.com",
		ChatModel:      "gpt-5.2",
		EmbeddingModel: "text-embedding-3-large",
		ImageModel:     "gpt-image-1",
		TTSModel:       "gpt-4o-mini-tts",
		STTModel:       "gpt-4o-transcribe",
		Voice:          "alloy",
	}, zap.NewNop())

	require.NotNil(t, profile)
	assert.Equal(t, "openai", profile.Name)
	assert.NotNil(t, profile.Chat)
	assert.NotNil(t, profile.Embedding)
	assert.NotNil(t, profile.Image)
	assert.NotNil(t, profile.TTS)
	assert.NotNil(t, profile.STT)
	assert.Equal(t, "gpt-5.2", profile.ModelForLanguage("zh-CN", "fallback"))
}

func TestNewGeminiProfile_AggregatesCapabilities(t *testing.T) {
	profile := NewGeminiProfile(GeminiConfig{
		APIKey:         "sk-gemini",
		BaseURL:        "https://generativelanguage.googleapis.com",
		ChatModel:      "gemini-2.5-flash",
		EmbeddingModel: "text-embedding-004",
		ImageModel:     "imagen-4",
		VideoModel:     "veo-3",
	}, zap.NewNop())

	require.NotNil(t, profile)
	assert.Equal(t, "gemini", profile.Name)
	assert.NotNil(t, profile.Chat)
	assert.NotNil(t, profile.Embedding)
	assert.NotNil(t, profile.Image)
	assert.NotNil(t, profile.Video)
	assert.Nil(t, profile.TTS)
	assert.Equal(t, "gemini-2.5-flash", profile.ModelForLanguage("en-US", "fallback"))
}

func TestNewAnthropicProfile_IsChatFirst(t *testing.T) {
	profile := NewAnthropicProfile(AnthropicConfig{
		APIKey:    "sk-anthropic",
		BaseURL:   "https://api.anthropic.com",
		ChatModel: "claude-sonnet-4-20250514",
	}, zap.NewNop())

	require.NotNil(t, profile)
	assert.Equal(t, "anthropic", profile.Name)
	assert.NotNil(t, profile.Chat)
	assert.Nil(t, profile.Embedding)
	assert.Nil(t, profile.Image)
	assert.Nil(t, profile.Video)
	assert.Equal(t, "claude-sonnet-4-20250514", profile.ModelForLanguage("zh", "fallback"))
}

func TestProfile_ModelForLanguage_Fallbacks(t *testing.T) {
	profile := &Profile{
		LanguageModels: map[string]string{
			"default": "default-model",
			"en":      "english-model",
		},
	}

	assert.Equal(t, "english-model", profile.ModelForLanguage("en-US", "fallback"))
	assert.Equal(t, "default-model", profile.ModelForLanguage("fr-FR", "fallback"))
	assert.Equal(t, "fallback", (*Profile)(nil).ModelForLanguage("en", "fallback"))
}

package multimodal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestBuildProvidersFromConfig_Defaults(t *testing.T) {
	result := BuildProvidersFromConfig(ProviderBuilderConfig{
		OpenAIAPIKey: "openai-key",
		GoogleAPIKey: "google-key",
	}, zap.NewNop())

	assert.Contains(t, result.ImageProviders, "openai")
	assert.Contains(t, result.ImageProviders, "gemini")
	assert.Contains(t, result.VideoProviders, "veo")
	assert.Equal(t, "openai", result.DefaultImage)
	assert.Equal(t, "veo", result.DefaultVideo)
}

func TestBuildProvidersFromConfig_InvalidDefaultsFallbackToAuto(t *testing.T) {
	result := BuildProvidersFromConfig(ProviderBuilderConfig{
		OpenAIAPIKey:         "openai-key",
		DefaultImageProvider: "non-existent",
		DefaultVideoProvider: "non-existent",
	}, zap.NewNop())

	assert.Contains(t, result.ImageProviders, "openai")
	assert.Empty(t, result.VideoProviders)
	assert.Equal(t, "", result.DefaultImage)
	assert.Equal(t, "", result.DefaultVideo)
}

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

// TestBuildProvidersFromConfig_OpenAIKeyEnablesSora 验证仅配置 openai_api_key 时也会注册 sora 视频（同一 Key 双用）。
func TestBuildProvidersFromConfig_OpenAIKeyEnablesSora(t *testing.T) {
	result := BuildProvidersFromConfig(ProviderBuilderConfig{
		OpenAIAPIKey: "openai-key",
	}, zap.NewNop())

	assert.Contains(t, result.ImageProviders, "openai")
	assert.Contains(t, result.VideoProviders, "sora")
}

func TestBuildProvidersFromConfig_InvalidDefaultsFallbackToAuto(t *testing.T) {
	// 使用仅图像厂商（如 flux），避免 OpenAI/Google 等同时注册视频导致 VideoProviders 非空
	result := BuildProvidersFromConfig(ProviderBuilderConfig{
		FluxAPIKey:           "flux-key",
		DefaultImageProvider: "non-existent",
		DefaultVideoProvider: "non-existent",
	}, zap.NewNop())

	assert.Contains(t, result.ImageProviders, "flux")
	assert.Empty(t, result.VideoProviders)
	assert.Equal(t, "", result.DefaultImage)
	assert.Equal(t, "", result.DefaultVideo)
}

package multimodal

import (
	"strings"

	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	"github.com/BaSui01/agentflow/llm/providers"
	vendorprofile "github.com/BaSui01/agentflow/llm/providers/vendor"
	"go.uber.org/zap"
)

// ProviderBuilderConfig 定义多模态 image/video provider 的构造输入。
type ProviderBuilderConfig struct {
	OpenAIAPIKey  string
	OpenAIBaseURL string

	GoogleAPIKey  string
	GoogleBaseURL string

	RunwayAPIKey  string
	RunwayBaseURL string

	VeoAPIKey  string
	VeoBaseURL string

	SoraAPIKey  string
	SoraBaseURL string

	KlingAPIKey  string
	KlingBaseURL string

	LumaAPIKey  string
	LumaBaseURL string

	MiniMaxAPIKey  string
	MiniMaxBaseURL string

	DefaultImageProvider string
	DefaultVideoProvider string
}

// ProviderBuilderResult 返回已构建的多模态 provider 集合与默认项。
type ProviderBuilderResult struct {
	ImageProviders map[string]image.Provider
	VideoProviders map[string]video.Provider
	DefaultImage   string
	DefaultVideo   string
}

// BuildProvidersFromConfig 统一构造多模态 image/video providers（单一构造入口）。
func BuildProvidersFromConfig(cfg ProviderBuilderConfig, logger *zap.Logger) ProviderBuilderResult {
	if logger == nil {
		logger = zap.NewNop()
	}

	result := ProviderBuilderResult{
		ImageProviders: make(map[string]image.Provider),
		VideoProviders: make(map[string]video.Provider),
		DefaultImage:   strings.TrimSpace(cfg.DefaultImageProvider),
		DefaultVideo:   strings.TrimSpace(cfg.DefaultVideoProvider),
	}

	if cfg.OpenAIAPIKey != "" {
		openaiProfile := vendorprofile.NewOpenAIProfile(vendorprofile.OpenAIConfig{
			APIKey:  cfg.OpenAIAPIKey,
			BaseURL: cfg.OpenAIBaseURL,
		}, logger)
		result.ImageProviders["openai"] = openaiProfile.Image
		if result.DefaultImage == "" {
			result.DefaultImage = "openai"
		}
	}

	if cfg.GoogleAPIKey != "" {
		geminiProfile := vendorprofile.NewGeminiProfile(vendorprofile.GeminiConfig{
			APIKey:  cfg.GoogleAPIKey,
			BaseURL: cfg.GoogleBaseURL,
		}, logger)
		result.ImageProviders["gemini"] = geminiProfile.Image
		result.VideoProviders["veo"] = geminiProfile.Video
		if result.DefaultImage == "" {
			result.DefaultImage = "gemini"
		}
		if result.DefaultVideo == "" {
			result.DefaultVideo = "veo"
		}
	}

	if cfg.VeoAPIKey != "" {
		result.VideoProviders["veo"] = video.NewVeoProvider(video.VeoConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.VeoAPIKey,
				BaseURL: cfg.VeoBaseURL,
			},
		}, logger)
		if result.DefaultVideo == "" {
			result.DefaultVideo = "veo"
		}
	}

	if cfg.RunwayAPIKey != "" {
		result.VideoProviders["runway"] = video.NewRunwayProvider(video.RunwayConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.RunwayAPIKey,
				BaseURL: cfg.RunwayBaseURL,
			},
		}, logger)
		if result.DefaultVideo == "" {
			result.DefaultVideo = "runway"
		}
	}

	if cfg.SoraAPIKey != "" {
		result.VideoProviders["sora"] = video.NewSoraProvider(video.SoraConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.SoraAPIKey,
				BaseURL: cfg.SoraBaseURL,
			},
		}, logger)
		if result.DefaultVideo == "" {
			result.DefaultVideo = "sora"
		}
	}

	if cfg.KlingAPIKey != "" {
		result.VideoProviders["kling"] = video.NewKlingProvider(video.KlingConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.KlingAPIKey,
				BaseURL: cfg.KlingBaseURL,
			},
		}, logger)
		if result.DefaultVideo == "" {
			result.DefaultVideo = "kling"
		}
	}

	if cfg.LumaAPIKey != "" {
		result.VideoProviders["luma"] = video.NewLumaProvider(video.LumaConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.LumaAPIKey,
				BaseURL: cfg.LumaBaseURL,
			},
		}, logger)
		if result.DefaultVideo == "" {
			result.DefaultVideo = "luma"
		}
	}

	if cfg.MiniMaxAPIKey != "" {
		result.VideoProviders["minimax-video"] = video.NewMiniMaxVideoProvider(video.MiniMaxVideoConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.MiniMaxAPIKey,
				BaseURL: cfg.MiniMaxBaseURL,
			},
		}, logger)
		if result.DefaultVideo == "" {
			result.DefaultVideo = "minimax-video"
		}
	}

	if result.DefaultImage != "" {
		if _, ok := result.ImageProviders[result.DefaultImage]; !ok {
			logger.Warn("configured default multimodal image provider is unavailable, fallback to auto selection",
				zap.String("provider", result.DefaultImage))
			result.DefaultImage = ""
		}
	}

	if result.DefaultVideo != "" {
		if _, ok := result.VideoProviders[result.DefaultVideo]; !ok {
			logger.Warn("configured default multimodal video provider is unavailable, fallback to auto selection",
				zap.String("provider", result.DefaultVideo))
			result.DefaultVideo = ""
		}
	}

	return result
}

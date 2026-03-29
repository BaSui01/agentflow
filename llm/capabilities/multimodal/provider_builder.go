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

	FluxAPIKey  string
	FluxBaseURL string

	StabilityAPIKey  string
	StabilityBaseURL string

	IdeogramAPIKey  string
	IdeogramBaseURL string

	TongyiAPIKey  string
	TongyiBaseURL string

	ZhipuAPIKey  string
	ZhipuBaseURL string

	BaiduAPIKey    string
	BaiduSecretKey string
	BaiduBaseURL   string

	DoubaoAPIKey  string
	DoubaoBaseURL string

	TencentSecretId  string
	TencentSecretKey string
	TencentBaseURL   string

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

	SeedanceAPIKey  string
	SeedanceBaseURL string

	DefaultImageProvider string
	DefaultVideoProvider string
}

// ProviderBuilderResult 返回已构建的多模态 provider 集合与默认项。
type ProviderBuilderResult struct {
	ImageProviders map[string]image.Provider
	VideoProviders map[string]video.Provider
	DefaultImage   string
	DefaultVideo   string
	// Profiles 保留按供应商聚合的能力档案，便于上层复用同一 vendor 的默认模型与能力集。
	Profiles map[string]*vendorprofile.Profile
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
		Profiles:       make(map[string]*vendorprofile.Profile),
	}

	if cfg.OpenAIAPIKey != "" {
		openaiProfile := vendorprofile.NewOpenAIProfile(vendorprofile.OpenAIConfig{
			APIKey:  cfg.OpenAIAPIKey,
			BaseURL: cfg.OpenAIBaseURL,
		}, logger)
		result.Profiles["openai"] = openaiProfile
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
		result.Profiles["gemini"] = geminiProfile
		result.ImageProviders["gemini"] = geminiProfile.Image
		result.VideoProviders["veo"] = geminiProfile.Video
		if result.DefaultImage == "" {
			result.DefaultImage = "gemini"
		}
		if result.DefaultVideo == "" {
			result.DefaultVideo = "veo"
		}
	}

	if cfg.FluxAPIKey != "" {
		result.ImageProviders["flux"] = image.NewFluxProvider(image.FluxConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.FluxAPIKey,
				BaseURL: cfg.FluxBaseURL,
			},
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "flux"
		}
	}

	if cfg.StabilityAPIKey != "" {
		result.ImageProviders["stability"] = image.NewStabilityProvider(image.StabilityConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.StabilityAPIKey,
				BaseURL: cfg.StabilityBaseURL,
			},
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "stability"
		}
	}

	if cfg.IdeogramAPIKey != "" {
		result.ImageProviders["ideogram"] = image.NewIdeogramProvider(image.IdeogramConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.IdeogramAPIKey,
				BaseURL: cfg.IdeogramBaseURL,
			},
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "ideogram"
		}
	}

	if cfg.TongyiAPIKey != "" {
		result.ImageProviders["tongyi"] = image.NewTongyiProvider(image.TongyiConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.TongyiAPIKey,
				BaseURL: cfg.TongyiBaseURL,
			},
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "tongyi"
		}
	}

	if cfg.ZhipuAPIKey != "" {
		result.ImageProviders["zhipu"] = image.NewZhipuProvider(image.ZhipuConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.ZhipuAPIKey,
				BaseURL: cfg.ZhipuBaseURL,
			},
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "zhipu"
		}
	}

	if cfg.BaiduAPIKey != "" && cfg.BaiduSecretKey != "" {
		result.ImageProviders["baidu"] = image.NewBaiduProvider(image.BaiduConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.BaiduAPIKey,
				BaseURL: cfg.BaiduBaseURL,
			},
			SecretKey: cfg.BaiduSecretKey,
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "baidu"
		}
	}

	if cfg.DoubaoAPIKey != "" {
		result.ImageProviders["doubao"] = image.NewDoubaoProvider(image.DoubaoConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.DoubaoAPIKey,
				BaseURL: cfg.DoubaoBaseURL,
			},
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "doubao"
		}
	}

	if cfg.TencentSecretId != "" && cfg.TencentSecretKey != "" {
		result.ImageProviders["tencent"] = image.NewTencentHunyuanProvider(image.TencentHunyuanConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.TencentSecretId,
				BaseURL: cfg.TencentBaseURL,
			},
			SecretKey: cfg.TencentSecretKey,
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "tencent"
		}
	}

	if cfg.KlingAPIKey != "" {
		result.ImageProviders["kling"] = image.NewKlingProvider(image.KlingConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.KlingAPIKey,
				BaseURL: cfg.KlingBaseURL,
			},
		})
		if result.DefaultImage == "" {
			result.DefaultImage = "kling"
		}
	}

	// 当配置了 VeoAPIKey 时覆盖由 GoogleAPIKey 注册的 veo，优先使用独立 Veo 端点。
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

	// OpenAI 与 Sora 共用同一 API Key（platform.openai.com）；未单独配置 SoraAPIKey 时使用 OpenAIAPIKey 注册 Sora。
	soraKey := cfg.SoraAPIKey
	if soraKey == "" {
		soraKey = cfg.OpenAIAPIKey
	}
	soraBase := cfg.SoraBaseURL
	if soraBase == "" {
		soraBase = cfg.OpenAIBaseURL
	}
	if soraKey != "" {
		result.VideoProviders["sora"] = video.NewSoraProvider(video.SoraConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  soraKey,
				BaseURL: soraBase,
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

	if cfg.SeedanceAPIKey != "" {
		result.VideoProviders["seedance"] = video.NewSeedanceProvider(video.SeedanceConfig{
			BaseProviderConfig: providers.BaseProviderConfig{
				APIKey:  cfg.SeedanceAPIKey,
				BaseURL: cfg.SeedanceBaseURL,
			},
		}, logger)
		if result.DefaultVideo == "" {
			result.DefaultVideo = "seedance"
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

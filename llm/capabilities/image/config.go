package image

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// OpenAIConfig 配置 OpenAI DALL-E 提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、BaseURL、Model、Timeout 字段。
type OpenAIConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// FluxConfig 配置 Black Forest Labs Flux 提供者.
type FluxConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// StabilityConfig 配置 Stability AI 提供者.
type StabilityConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// IdeogramConfig 配置 Ideogram 图像提供者.
type IdeogramConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// TongyiConfig 配置阿里云通义万相图像提供者.
type TongyiConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// ZhipuConfig 配置智谱 AI 图像提供者.
type ZhipuConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// BaiduConfig 配置百度文心 ERNIE-ViLG 图像提供者；需 APIKey（client_id）与 SecretKey（client_secret）.
type BaiduConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
	SecretKey                    string `yaml:"secret_key" json:"-"`
}

// DoubaoConfig 配置火山引擎/豆包图像提供者.
type DoubaoConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// TencentHunyuanConfig 配置腾讯混元生图；需 SecretId（APIKey）+ SecretKey，使用 TC3-HMAC-SHA256 签名.
type TencentHunyuanConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
	SecretKey                    string `yaml:"secret_key" json:"-"`
}

// KlingConfig 配置可灵 Kling 图像提供者（与视频共用 api.klingai.com，可选同一 API Key）.
type KlingConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultOpenAIConfig 返回默认 OpenAI 图像配置.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com",
			Model:   "dall-e-3",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultFluxConfig 返回默认 Flux 配置.
func DefaultFluxConfig() FluxConfig {
	return FluxConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.bfl.ml",
			Model:   "flux-1.1-pro",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultStabilityConfig 返回默认 Stability AI 配置.
func DefaultStabilityConfig() StabilityConfig {
	return StabilityConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.stability.ai",
			Model:   "stable-diffusion-3.5-large",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultIdeogramConfig 返回默认 Ideogram 配置.
func DefaultIdeogramConfig() IdeogramConfig {
	return IdeogramConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.ideogram.ai",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultTongyiConfig 返回默认通义万相配置.
func DefaultTongyiConfig() TongyiConfig {
	return TongyiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://dashscope.aliyuncs.com",
			Model:   "wanx2.1-t2i-turbo",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultZhipuConfig 返回默认智谱配置.
func DefaultZhipuConfig() ZhipuConfig {
	return ZhipuConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://open.bigmodel.cn",
			Model:   "glm-image",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultBaiduConfig 返回默认百度文心配置.
func DefaultBaiduConfig() BaiduConfig {
	return BaiduConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://aip.baidubce.com",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultDoubaoConfig 返回默认豆包/火山配置.
func DefaultDoubaoConfig() DoubaoConfig {
	return DoubaoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://ark.cn-beijing.volces.com",
			Model:   "doubao-seedream-4-0-250828",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultTencentHunyuanConfig 返回默认腾讯混元生图配置（TC3 签名已实现）.
func DefaultTencentHunyuanConfig() TencentHunyuanConfig {
	return TencentHunyuanConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://aiart.tencentcloudapi.com",
			Timeout: 120 * time.Second,
		},
	}
}

// DefaultKlingConfig 返回默认可灵图像配置（与视频共用 BaseURL，异步任务+轮询）.
func DefaultKlingConfig() KlingConfig {
	return KlingConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.klingai.com",
			Model:   "kling-v1-5",
			Timeout: 120 * time.Second,
		},
	}
}

// GeminiConfig 配置 Google Gemini 图像生成提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、Model、Timeout 字段。
type GeminiConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultGeminiConfig 返回默认 Gemini 图像配置.
func DefaultGeminiConfig() GeminiConfig {
	return GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "gemini-3-pro-image-preview",
			Timeout: 120 * time.Second,
		},
	}
}

// Imagen4Config 配置 Google Imagen 4 提供者.
type Imagen4Config struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultImagen4Config 返回默认 Imagen 4 配置.
func DefaultImagen4Config() Imagen4Config {
	return Imagen4Config{
		BaseProviderConfig: providers.BaseProviderConfig{
			Model:   "imagen-4.0-generate-preview",
			Timeout: 120 * time.Second,
		},
	}
}


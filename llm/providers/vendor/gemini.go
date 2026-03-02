package vendor

import (
	"time"

	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	"github.com/BaSui01/agentflow/llm/providers"
	llmgemini "github.com/BaSui01/agentflow/llm/providers/gemini"
	"go.uber.org/zap"
)

type GeminiConfig struct {
	APIKey         string
	BaseURL        string
	ChatModel      string
	EmbeddingModel string
	ImageModel     string
	VideoModel     string
	Timeout        time.Duration
	ProjectID      string
	Region         string
}

func NewGeminiProfile(cfg GeminiConfig, logger *zap.Logger) *Profile {
	chat := llmgemini.NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.ChatModel,
			Timeout: cfg.Timeout,
		},
		ProjectID: cfg.ProjectID,
		Region:    cfg.Region,
	}, logger)

	embed := embedding.NewGeminiProvider(embedding.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.EmbeddingModel,
			Timeout: cfg.Timeout,
		},
	})
	img := image.NewGeminiProvider(image.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.ImageModel,
			Timeout: cfg.Timeout,
		},
	})
	vid := video.NewVeoProvider(video.VeoConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.VideoModel,
			Timeout: cfg.Timeout,
		},
	}, logger)

	return &Profile{
		Name:      "gemini",
		Chat:      chat,
		Embedding: embed,
		Image:     img,
		Video:     vid,
		LanguageModels: map[string]string{
			"default": cfg.ChatModel,
			"zh":      cfg.ChatModel,
			"en":      cfg.ChatModel,
		},
	}
}

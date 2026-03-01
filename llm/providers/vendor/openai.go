package vendor

import (
	"time"

	"github.com/BaSui01/agentflow/llm/embedding"
	"github.com/BaSui01/agentflow/llm/image"
	"github.com/BaSui01/agentflow/llm/providers"
	llmopenai "github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/llm/speech"
	"go.uber.org/zap"
)

type OpenAIConfig struct {
	APIKey       string
	BaseURL      string
	ChatModel    string
	EmbeddingModel string
	ImageModel   string
	TTSModel     string
	STTModel     string
	Voice        string
	Timeout      time.Duration
	UseResponsesAPI bool
}

func NewOpenAIProfile(cfg OpenAIConfig, logger *zap.Logger) *Profile {
	chat := llmopenai.NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.ChatModel,
			Timeout: cfg.Timeout,
		},
		UseResponsesAPI: cfg.UseResponsesAPI,
	}, logger)

	embed := embedding.NewOpenAIProvider(embedding.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.EmbeddingModel,
			Timeout: cfg.Timeout,
		},
	})
	img := image.NewOpenAIProvider(image.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.ImageModel,
			Timeout: cfg.Timeout,
		},
	})
	tts := speech.NewOpenAITTSProvider(speech.OpenAITTSConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.TTSModel,
			Timeout: cfg.Timeout,
		},
		Voice: cfg.Voice,
	})
	stt := speech.NewOpenAISTTProvider(speech.OpenAISTTConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  cfg.APIKey,
			BaseURL: cfg.BaseURL,
			Model:   cfg.STTModel,
			Timeout: cfg.Timeout,
		},
	})

	return &Profile{
		Name:      "openai",
		Chat:      chat,
		Embedding: embed,
		Image:     img,
		TTS:       tts,
		STT:       stt,
		LanguageModels: map[string]string{
			"default": cfg.ChatModel,
			"zh":      cfg.ChatModel,
			"en":      cfg.ChatModel,
		},
	}
}

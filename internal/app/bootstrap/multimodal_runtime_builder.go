package bootstrap

import (
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/pkg/storage"
	"go.uber.org/zap"
)

// MultimodalRuntime groups multimodal handler and key runtime metadata.
type MultimodalRuntime struct {
	Handler            *handlers.MultimodalHandler
	ReferenceBackend   string
	ImageProviderCount int
	VideoProviderCount int
}

// ValidateMultimodalReferenceBackend validates and normalizes multimodal reference backend.
func ValidateMultimodalReferenceBackend(cfg *config.Config) (string, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.Multimodal.ReferenceStoreBackend))
	if backend != "redis" {
		return "", fmt.Errorf("multimodal.reference_store_backend must be redis")
	}
	return backend, nil
}

// BuildMultimodalRuntime builds multimodal handler runtime from config.
// If multimodal is disabled, it returns nil.
func BuildMultimodalRuntime(
	cfg *config.Config,
	chatProvider llm.Provider,
	budgetManager *llmpolicy.TokenBudgetManager,
	ledger observability.Ledger,
	referenceStore storage.ReferenceStore,
	logger *zap.Logger,
) (*MultimodalRuntime, error) {
	if !cfg.Multimodal.Enabled {
		return nil, nil
	}

	backend, err := ValidateMultimodalReferenceBackend(cfg)
	if err != nil {
		return nil, err
	}

	builderConfig := multimodal.ProviderBuilderConfig{
		OpenAIAPIKey:         firstNonEmpty(cfg.Multimodal.Image.OpenAIAPIKey, cfg.LLM.APIKey),
		OpenAIBaseURL:        firstNonEmpty(cfg.Multimodal.Image.OpenAIBaseURL, cfg.LLM.BaseURL),
		GoogleAPIKey:         firstNonEmpty(cfg.Multimodal.Video.GoogleAPIKey, cfg.Multimodal.Image.GeminiAPIKey),
		GoogleBaseURL:        cfg.Multimodal.Video.GoogleBaseURL,
		FluxAPIKey:           cfg.Multimodal.Image.FluxAPIKey,
		FluxBaseURL:          cfg.Multimodal.Image.FluxBaseURL,
		StabilityAPIKey:      cfg.Multimodal.Image.StabilityAPIKey,
		StabilityBaseURL:     cfg.Multimodal.Image.StabilityBaseURL,
		IdeogramAPIKey:       cfg.Multimodal.Image.IdeogramAPIKey,
		IdeogramBaseURL:      cfg.Multimodal.Image.IdeogramBaseURL,
		TongyiAPIKey:         cfg.Multimodal.Image.TongyiAPIKey,
		TongyiBaseURL:        cfg.Multimodal.Image.TongyiBaseURL,
		ZhipuAPIKey:         cfg.Multimodal.Image.ZhipuAPIKey,
		ZhipuBaseURL:        cfg.Multimodal.Image.ZhipuBaseURL,
		BaiduAPIKey:         cfg.Multimodal.Image.BaiduAPIKey,
		BaiduSecretKey:      cfg.Multimodal.Image.BaiduSecretKey,
		BaiduBaseURL:        cfg.Multimodal.Image.BaiduBaseURL,
		DoubaoAPIKey:        cfg.Multimodal.Image.DoubaoAPIKey,
		DoubaoBaseURL:       cfg.Multimodal.Image.DoubaoBaseURL,
		TencentSecretId:     cfg.Multimodal.Image.TencentSecretId,
		TencentSecretKey:    cfg.Multimodal.Image.TencentSecretKey,
		TencentBaseURL:      cfg.Multimodal.Image.TencentBaseURL,
		RunwayAPIKey:        cfg.Multimodal.Video.RunwayAPIKey,
		RunwayBaseURL:       cfg.Multimodal.Video.RunwayBaseURL,
		VeoAPIKey:           cfg.Multimodal.Video.VeoAPIKey,
		VeoBaseURL:          cfg.Multimodal.Video.VeoBaseURL,
		SoraAPIKey:          cfg.Multimodal.Video.SoraAPIKey,
		SoraBaseURL:         cfg.Multimodal.Video.SoraBaseURL,
		KlingAPIKey:         cfg.Multimodal.Video.KlingAPIKey,
		KlingBaseURL:        cfg.Multimodal.Video.KlingBaseURL,
		LumaAPIKey:          cfg.Multimodal.Video.LumaAPIKey,
		LumaBaseURL:         cfg.Multimodal.Video.LumaBaseURL,
		MiniMaxAPIKey:       cfg.Multimodal.Video.MiniMaxAPIKey,
		MiniMaxBaseURL:      cfg.Multimodal.Video.MiniMaxBaseURL,
		SeedanceAPIKey:      cfg.Multimodal.Video.SeedanceAPIKey,
		SeedanceBaseURL:     cfg.Multimodal.Video.SeedanceBaseURL,
		DefaultImageProvider: cfg.Multimodal.DefaultImageProvider,
		DefaultVideoProvider: cfg.Multimodal.DefaultVideoProvider,
	}
	providerSet := multimodal.BuildProvidersFromConfig(builderConfig, logger)

	pipeline := &multimodal.DefaultPromptPipeline{}
	handler := handlers.NewMultimodalHandlerWithProviders(
		chatProvider,
		llmpolicy.NewManager(llmpolicy.ManagerConfig{Budget: budgetManager}),
		ledger,
		providerSet.ImageProviders,
		providerSet.VideoProviders,
		providerSet.DefaultImage,
		providerSet.DefaultVideo,
		pipeline,
		cfg.Multimodal.ReferenceMaxSizeBytes,
		cfg.Multimodal.ReferenceTTL,
		referenceStore,
		firstNonEmpty(cfg.Multimodal.DefaultChatModel, cfg.Agent.Model),
		logger,
	)

	return &MultimodalRuntime{
		Handler:            handler,
		ReferenceBackend:   backend,
		ImageProviderCount: len(providerSet.ImageProviders),
		VideoProviderCount: len(providerSet.VideoProviders),
	}, nil
}

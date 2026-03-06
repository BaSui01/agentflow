package bootstrap

import (
	"fmt"
	"strings"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
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
	referenceStore handlers.ReferenceStore,
	logger *zap.Logger,
) (*MultimodalRuntime, error) {
	if !cfg.Multimodal.Enabled {
		return nil, nil
	}

	backend, err := ValidateMultimodalReferenceBackend(cfg)
	if err != nil {
		return nil, err
	}

	multimodalCfg := handlers.MultimodalHandlerConfig{
		ChatProvider:         chatProvider,
		PolicyManager:        llmpolicy.NewManager(llmpolicy.ManagerConfig{Budget: budgetManager}),
		Ledger:               ledger,
		OpenAIAPIKey:         firstNonEmpty(cfg.Multimodal.Image.OpenAIAPIKey, cfg.LLM.APIKey),
		OpenAIBaseURL:        firstNonEmpty(cfg.Multimodal.Image.OpenAIBaseURL, cfg.LLM.BaseURL),
		GoogleAPIKey:         firstNonEmpty(cfg.Multimodal.Video.GoogleAPIKey, cfg.Multimodal.Image.GeminiAPIKey),
		GoogleBaseURL:        cfg.Multimodal.Video.GoogleBaseURL,
		RunwayAPIKey:         cfg.Multimodal.Video.RunwayAPIKey,
		RunwayBaseURL:        cfg.Multimodal.Video.RunwayBaseURL,
		VeoAPIKey:            cfg.Multimodal.Video.VeoAPIKey,
		VeoBaseURL:           cfg.Multimodal.Video.VeoBaseURL,
		SoraAPIKey:           cfg.Multimodal.Video.SoraAPIKey,
		SoraBaseURL:          cfg.Multimodal.Video.SoraBaseURL,
		KlingAPIKey:          cfg.Multimodal.Video.KlingAPIKey,
		KlingBaseURL:         cfg.Multimodal.Video.KlingBaseURL,
		LumaAPIKey:           cfg.Multimodal.Video.LumaAPIKey,
		LumaBaseURL:          cfg.Multimodal.Video.LumaBaseURL,
		MiniMaxAPIKey:        cfg.Multimodal.Video.MiniMaxAPIKey,
		MiniMaxBaseURL:       cfg.Multimodal.Video.MiniMaxBaseURL,
		DefaultImageProvider: cfg.Multimodal.DefaultImageProvider,
		DefaultVideoProvider: cfg.Multimodal.DefaultVideoProvider,
		ReferenceMaxSize:     cfg.Multimodal.ReferenceMaxSizeBytes,
		ReferenceTTL:         cfg.Multimodal.ReferenceTTL,
		ReferenceStore:       referenceStore,
	}

	imageProviderCount := 0
	videoProviderCount := 0
	if multimodalCfg.OpenAIAPIKey != "" {
		imageProviderCount++
	}
	if multimodalCfg.GoogleAPIKey != "" {
		imageProviderCount++
		videoProviderCount++
	}
	if multimodalCfg.RunwayAPIKey != "" {
		videoProviderCount++
	}
	if multimodalCfg.VeoAPIKey != "" && multimodalCfg.GoogleAPIKey == "" {
		videoProviderCount++
	}
	if multimodalCfg.SoraAPIKey != "" {
		videoProviderCount++
	}
	if multimodalCfg.KlingAPIKey != "" {
		videoProviderCount++
	}
	if multimodalCfg.LumaAPIKey != "" {
		videoProviderCount++
	}
	if multimodalCfg.MiniMaxAPIKey != "" {
		videoProviderCount++
	}

	return &MultimodalRuntime{
		Handler:            handlers.NewMultimodalHandlerFromConfig(multimodalCfg, logger),
		ReferenceBackend:   backend,
		ImageProviderCount: imageProviderCount,
		VideoProviderCount: videoProviderCount,
	}, nil
}

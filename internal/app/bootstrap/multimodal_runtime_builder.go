package bootstrap

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm/capabilities"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	"github.com/BaSui01/agentflow/pkg/storage"
	"go.uber.org/zap"
)

const (
	defaultMultimodalReferenceBytes = 8 << 20 // 8MB
	defaultMultimodalReferenceTTL   = 2 * time.Hour
	defaultMultimodalChatModel      = "gpt-4o-mini"
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
		ZhipuAPIKey:          cfg.Multimodal.Image.ZhipuAPIKey,
		ZhipuBaseURL:         cfg.Multimodal.Image.ZhipuBaseURL,
		BaiduAPIKey:          cfg.Multimodal.Image.BaiduAPIKey,
		BaiduSecretKey:       cfg.Multimodal.Image.BaiduSecretKey,
		BaiduBaseURL:         cfg.Multimodal.Image.BaiduBaseURL,
		DoubaoAPIKey:         cfg.Multimodal.Image.DoubaoAPIKey,
		DoubaoBaseURL:        cfg.Multimodal.Image.DoubaoBaseURL,
		TencentSecretId:      cfg.Multimodal.Image.TencentSecretId,
		TencentSecretKey:     cfg.Multimodal.Image.TencentSecretKey,
		TencentBaseURL:       cfg.Multimodal.Image.TencentBaseURL,
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
		SeedanceAPIKey:       cfg.Multimodal.Video.SeedanceAPIKey,
		SeedanceBaseURL:      cfg.Multimodal.Video.SeedanceBaseURL,
		DefaultImageProvider: cfg.Multimodal.DefaultImageProvider,
		DefaultVideoProvider: cfg.Multimodal.DefaultVideoProvider,
	}
	providerSet := multimodal.BuildProvidersFromConfig(builderConfig, logger)

	router := multimodal.NewRouter()
	imageNames := make([]string, 0, len(providerSet.ImageProviders))
	for name := range providerSet.ImageProviders {
		imageNames = append(imageNames, name)
	}
	sort.Strings(imageNames)
	videoNames := make([]string, 0, len(providerSet.VideoProviders))
	for name := range providerSet.VideoProviders {
		videoNames = append(videoNames, name)
	}
	sort.Strings(videoNames)

	defaultImageProvider := strings.TrimSpace(providerSet.DefaultImage)
	if defaultImageProvider == "" && len(imageNames) > 0 {
		defaultImageProvider = imageNames[0]
	}
	defaultVideoProvider := strings.TrimSpace(providerSet.DefaultVideo)
	if defaultVideoProvider == "" && len(videoNames) > 0 {
		defaultVideoProvider = videoNames[0]
	}

	for _, name := range imageNames {
		router.RegisterImage(name, providerSet.ImageProviders[name], name == defaultImageProvider)
	}
	for _, name := range videoNames {
		router.RegisterVideo(name, providerSet.VideoProviders[name], name == defaultVideoProvider)
	}

	policyManager := llmpolicy.NewManager(llmpolicy.ManagerConfig{Budget: budgetManager})
	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider:  chatProvider,
		Capabilities:  capabilities.NewEntry(router),
		PolicyManager: policyManager,
		Ledger:        ledger,
		Logger:        logger,
	})

	referenceMaxSize := cfg.Multimodal.ReferenceMaxSizeBytes
	if referenceMaxSize <= 0 {
		referenceMaxSize = defaultMultimodalReferenceBytes
	}
	referenceTTL := cfg.Multimodal.ReferenceTTL
	if referenceTTL <= 0 {
		referenceTTL = defaultMultimodalReferenceTTL
	}
	if referenceStore == nil {
		referenceStore = storage.NewMemoryReferenceStore()
	}

	defaultChatModel := firstNonEmpty(cfg.Multimodal.DefaultChatModel, cfg.Agent.Model, defaultMultimodalChatModel)
	pipeline := &multimodal.DefaultPromptPipeline{}
	service := usecase.NewDefaultMultimodalService(
		usecase.MultimodalRuntime{
			Gateway:              gateway,
			Pipeline:             pipeline,
			ResolveImageProvider: newMultimodalImageProviderResolver(router, defaultImageProvider),
			ResolveVideoProvider: newMultimodalVideoProviderResolver(router, defaultVideoProvider),
			ReferenceStore:       referenceStore,
			ReferenceTTL:         referenceTTL,
			ReferenceMaxSize:     referenceMaxSize,
			ChatEnabled:          chatProvider != nil,
			DefaultChatModel:     defaultChatModel,
		},
	)

	handler := handlers.NewMultimodalHandler(service, logger)
	handler.ApplyRuntimeDeps(handlers.MultimodalHandlerRuntimeDeps{
		DefaultImageProvider: defaultImageProvider,
		DefaultVideoProvider: defaultVideoProvider,
		ImageProviders:       imageNames,
		VideoProviders:       videoNames,
		ReferenceMaxSize:     referenceMaxSize,
		ReferenceTTL:         referenceTTL,
		ReferenceStore:       referenceStore,
		ChatEnabled:          chatProvider != nil,
		ResolveImageProvider: newMultimodalImageProviderResolver(router, defaultImageProvider),
		ResolveVideoProvider: newMultimodalVideoProviderResolver(router, defaultVideoProvider),
		ImageStreamProvider:  newMultimodalStreamingImageProviderLookup(router),
	})

	return &MultimodalRuntime{
		Handler:            handler,
		ReferenceBackend:   backend,
		ImageProviderCount: len(imageNames),
		VideoProviderCount: len(videoNames),
	}, nil
}

func newMultimodalImageProviderResolver(router *multimodal.Router, defaultProvider string) usecase.MultimodalProviderResolver {
	return func(provider string) (string, error) {
		name := strings.TrimSpace(provider)
		if name == "" {
			name = strings.TrimSpace(defaultProvider)
		}
		if name == "" {
			return "", fmt.Errorf("no default image provider available")
		}
		if _, err := router.Image(name); err != nil {
			return "", fmt.Errorf("image provider %q not found", name)
		}
		return name, nil
	}
}

func newMultimodalVideoProviderResolver(router *multimodal.Router, defaultProvider string) usecase.MultimodalProviderResolver {
	return func(provider string) (string, error) {
		name := strings.TrimSpace(provider)
		if name == "" {
			name = strings.TrimSpace(defaultProvider)
		}
		if name == "" {
			return "", fmt.Errorf("no default video provider available")
		}
		if _, err := router.Video(name); err != nil {
			return "", fmt.Errorf("video provider %q not found", name)
		}
		return name, nil
	}
}

func newMultimodalStreamingImageProviderLookup(router *multimodal.Router) func(string) (image.StreamingProvider, bool) {
	return func(provider string) (image.StreamingProvider, bool) {
		if router == nil {
			return nil, false
		}
		p, err := router.Image(provider)
		if err != nil {
			return nil, false
		}
		sp, ok := p.(image.StreamingProvider)
		return sp, ok
	}
}

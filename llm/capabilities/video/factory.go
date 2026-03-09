package video

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// NewProvider creates a video provider by name using the provided config.
// The cfg argument accepts either a concrete config value (e.g. SoraConfig),
// a pointer to that config type, or nil to use package defaults.
func NewProvider(name string, cfg any, logger *zap.Logger) (Provider, error) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "gemini", "gemini-video":
		c, err := resolveGeminiConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewGeminiProvider(c, logger), nil
	case "veo":
		c, err := resolveVeoConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewVeoProvider(c, logger), nil
	case "runway":
		c, err := resolveRunwayConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewRunwayProvider(c, logger), nil
	case "sora":
		c, err := resolveSoraConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewSoraProvider(c, logger), nil
	case "kling":
		c, err := resolveKlingConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewKlingProvider(c, logger), nil
	case "luma":
		c, err := resolveLumaConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewLumaProvider(c, logger), nil
	case "minimax", "minimax-video":
		c, err := resolveMiniMaxVideoConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewMiniMaxVideoProvider(c, logger), nil
	case "seedance", "即梦":
		c, err := resolveSeedanceConfig(cfg)
		if err != nil {
			return nil, err
		}
		return NewSeedanceProvider(c, logger), nil
	default:
		return nil, fmt.Errorf("unknown video provider %q", name)
	}
}

func resolveGeminiConfig(cfg any) (GeminiConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultGeminiConfig(), nil
	case GeminiConfig:
		return c, nil
	case *GeminiConfig:
		if c == nil {
			return DefaultGeminiConfig(), nil
		}
		return *c, nil
	default:
		return GeminiConfig{}, fmt.Errorf("invalid config type for gemini-video: %T", cfg)
	}
}

func resolveVeoConfig(cfg any) (VeoConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultVeoConfig(), nil
	case VeoConfig:
		return c, nil
	case *VeoConfig:
		if c == nil {
			return DefaultVeoConfig(), nil
		}
		return *c, nil
	default:
		return VeoConfig{}, fmt.Errorf("invalid config type for veo: %T", cfg)
	}
}

func resolveRunwayConfig(cfg any) (RunwayConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultRunwayConfig(), nil
	case RunwayConfig:
		return c, nil
	case *RunwayConfig:
		if c == nil {
			return DefaultRunwayConfig(), nil
		}
		return *c, nil
	default:
		return RunwayConfig{}, fmt.Errorf("invalid config type for runway: %T", cfg)
	}
}

func resolveSoraConfig(cfg any) (SoraConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultSoraConfig(), nil
	case SoraConfig:
		return c, nil
	case *SoraConfig:
		if c == nil {
			return DefaultSoraConfig(), nil
		}
		return *c, nil
	default:
		return SoraConfig{}, fmt.Errorf("invalid config type for sora: %T", cfg)
	}
}

func resolveKlingConfig(cfg any) (KlingConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultKlingConfig(), nil
	case KlingConfig:
		return c, nil
	case *KlingConfig:
		if c == nil {
			return DefaultKlingConfig(), nil
		}
		return *c, nil
	default:
		return KlingConfig{}, fmt.Errorf("invalid config type for kling: %T", cfg)
	}
}

func resolveLumaConfig(cfg any) (LumaConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultLumaConfig(), nil
	case LumaConfig:
		return c, nil
	case *LumaConfig:
		if c == nil {
			return DefaultLumaConfig(), nil
		}
		return *c, nil
	default:
		return LumaConfig{}, fmt.Errorf("invalid config type for luma: %T", cfg)
	}
}

func resolveMiniMaxVideoConfig(cfg any) (MiniMaxVideoConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultMiniMaxVideoConfig(), nil
	case MiniMaxVideoConfig:
		return c, nil
	case *MiniMaxVideoConfig:
		if c == nil {
			return DefaultMiniMaxVideoConfig(), nil
		}
		return *c, nil
	default:
		return MiniMaxVideoConfig{}, fmt.Errorf("invalid config type for minimax-video: %T", cfg)
	}
}

func resolveSeedanceConfig(cfg any) (SeedanceConfig, error) {
	switch c := cfg.(type) {
	case nil:
		return DefaultSeedanceConfig(), nil
	case SeedanceConfig:
		return c, nil
	case *SeedanceConfig:
		if c == nil {
			return DefaultSeedanceConfig(), nil
		}
		return *c, nil
	default:
		return SeedanceConfig{}, fmt.Errorf("invalid config type for seedance: %T", cfg)
	}
}

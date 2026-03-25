package bootstrap

import (
	"github.com/BaSui01/agentflow/config"
	"go.uber.org/zap"
)

// HotReloadRuntime groups hot-reload manager and config API handler.
type HotReloadRuntime struct {
	Manager    *config.HotReloadManager
	APIHandler *config.ConfigAPIHandler
}

// BuildHotReloadRuntime creates hot-reload manager and config API handler.
func BuildHotReloadRuntime(cfg *config.Config, configPath string, logger *zap.Logger) *HotReloadRuntime {
	opts := []config.HotReloadOption{
		config.WithHotReloadLogger(logger),
		config.WithMaxHistorySize(20),
		config.WithValidateFunc(func(newConfig *config.Config) error {
			return newConfig.Validate()
		}),
	}
	if configPath != "" {
		opts = append(opts, config.WithConfigPath(configPath))
	}

	manager := config.NewHotReloadManager(cfg, opts...)
	apiHandler := config.NewConfigAPIHandler(manager)
	apiHandler.SetLogger(logger)
	return &HotReloadRuntime{
		Manager:    manager,
		APIHandler: apiHandler,
	}
}

// RegisterHotReloadCallbacks wires standard startup callbacks for hot-reload manager.
func RegisterHotReloadCallbacks(
	manager *config.HotReloadManager,
	logger *zap.Logger,
	onReload func(oldConfig, newConfig *config.Config),
) {
	manager.OnChange(func(change config.ConfigChange) {
		logger.Info("Configuration changed",
			zap.String("path", change.Path),
			zap.String("source", change.Source),
			zap.Bool("requires_restart", change.RequiresRestart),
		)
	})

	manager.OnReload(func(oldConfig, newConfig *config.Config) {
		logger.Info("Configuration reloaded")
		if onReload != nil {
			onReload(oldConfig, newConfig)
		}
	})

	manager.OnRollback(func(event config.RollbackEvent) {
		logger.Warn("Configuration rolled back",
			zap.String("reason", event.Reason),
			zap.Int("version", event.Version),
		)
		if onReload != nil {
			onReload(event.FailedConfig, event.RestoredConfig)
		}
	})
}

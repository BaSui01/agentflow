package main

import (
	"github.com/BaSui01/agentflow/config"
	"go.uber.org/zap"
)

func (s *Server) initHotReloadManager() error {
	opts := []config.HotReloadOption{
		config.WithHotReloadLogger(s.logger),
	}

	if s.configPath != "" {
		opts = append(opts, config.WithConfigPath(s.configPath))
	}

	s.hotReloadManager = config.NewHotReloadManager(s.cfg, opts...)

	s.hotReloadManager.OnChange(func(change config.ConfigChange) {
		s.logger.Info("Configuration changed",
			zap.String("path", change.Path),
			zap.String("source", change.Source),
			zap.Bool("requires_restart", change.RequiresRestart),
		)
	})

	s.hotReloadManager.OnReload(func(oldConfig, newConfig *config.Config) {
		s.logger.Info("Configuration reloaded")
		s.cfg = newConfig
	})

	s.configAPIHandler = config.NewConfigAPIHandler(s.hotReloadManager)

	return nil
}

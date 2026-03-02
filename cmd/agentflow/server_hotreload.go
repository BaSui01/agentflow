package main

import (
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
)

func (s *Server) initHotReloadManager() error {
	runtime := bootstrap.BuildHotReloadRuntime(s.cfg, s.configPath, s.logger)
	s.hotReloadManager = runtime.Manager
	s.configAPIHandler = runtime.APIHandler

	bootstrap.RegisterHotReloadCallbacks(s.hotReloadManager, s.logger, func(_old, newConfig *config.Config) {
		s.cfg = newConfig
	})

	return nil
}

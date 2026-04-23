package main

import (
	"sort"

	"go.uber.org/zap"
)

type startupSummary struct {
	EnabledCapabilities   []string
	DisabledCapabilities  []string
	DependencyStatus      map[string]string
	RestartRequiredRoutes []string
	HotReloadEnabled      bool
	MetricsBindAddress    string
	PProfEnabled          bool
	ToolApprovalBackend   string
	MultimodalEnabled     bool
	MultimodalRefBackend  string
}

func (s *Server) startupSummary() startupSummary {
	summary := startupSummary{
		EnabledCapabilities:   make([]string, 0, 16),
		DisabledCapabilities:  make([]string, 0, 16),
		DependencyStatus:      make(map[string]string, 8),
		RestartRequiredRoutes: make([]string, 0, 8),
	}
	if s == nil || s.cfg == nil {
		return summary
	}

	summary.HotReloadEnabled = s.configPath != "" && s.ops.hotReloadManager != nil
	summary.MetricsBindAddress = s.cfg.Server.MetricsBindAddress
	summary.PProfEnabled = s.cfg.Server.EnablePProf
	summary.ToolApprovalBackend = s.cfg.HostedTools.Approval.Backend
	summary.MultimodalEnabled = s.cfg.Multimodal.Enabled
	summary.MultimodalRefBackend = s.cfg.Multimodal.ReferenceStoreBackend

	summary.EnabledCapabilities = append(summary.EnabledCapabilities,
		"http_api",
		"metrics",
	)
	if summary.HotReloadEnabled {
		summary.EnabledCapabilities = append(summary.EnabledCapabilities, "hot_reload")
	} else {
		summary.DisabledCapabilities = append(summary.DisabledCapabilities, "hot_reload")
	}
	if summary.PProfEnabled {
		summary.EnabledCapabilities = append(summary.EnabledCapabilities, "pprof")
	} else {
		summary.DisabledCapabilities = append(summary.DisabledCapabilities, "pprof")
	}

	appendCapabilityState := func(name string, enabled bool) {
		if enabled {
			summary.EnabledCapabilities = append(summary.EnabledCapabilities, name)
			return
		}
		summary.DisabledCapabilities = append(summary.DisabledCapabilities, name)
	}

	appendCapabilityState("chat", s.handlers.chatHandler != nil)
	appendCapabilityState("agent", s.handlers.agentHandler != nil)
	appendCapabilityState("health", s.handlers.healthHandler != nil)
	appendCapabilityState("protocol", s.handlers.protocolHandler != nil)
	appendCapabilityState("rag", s.handlers.ragHandler != nil)
	appendCapabilityState("workflow", s.handlers.workflowHandler != nil)
	appendCapabilityState("multimodal", s.handlers.multimodalHandler != nil)
	appendCapabilityState("cost", s.handlers.costHandler != nil)
	appendCapabilityState("api_key_management", s.handlers.apiKeyHandler != nil)
	appendCapabilityState("tool_registry", s.handlers.toolRegistryHandler != nil)
	appendCapabilityState("tool_provider_config", s.handlers.toolProviderHandler != nil)
	appendCapabilityState("tool_approval", s.handlers.toolApprovalHandler != nil)
	appendCapabilityState("mongo_runtime", s.infra.mongoClient != nil)
	appendCapabilityState("audit", s.infra.auditLogger != nil)
	appendCapabilityState("enhanced_memory", s.infra.enhancedMemory != nil)
	appendCapabilityState("ab_testing", s.infra.abTester != nil)

	summary.DependencyStatus["database"] = requiredDependencyStatus(s.infra.db != nil)
	summary.DependencyStatus["mongodb"] = requiredDependencyStatus(s.infra.mongoClient != nil)
	summary.DependencyStatus["llm_runtime"] = requiredDependencyStatus(s.text.provider != nil)

	if s.cfg.Multimodal.Enabled {
		summary.DependencyStatus["multimodal_redis"] = requiredDependencyStatus(s.infra.multimodalRedis != nil)
	} else {
		summary.DependencyStatus["multimodal_redis"] = "disabled"
	}

	switch s.cfg.HostedTools.Approval.Backend {
	case "redis":
		summary.DependencyStatus["tool_approval_store"] = requiredDependencyStatus(s.infra.toolApprovalRedis != nil)
	default:
		summary.DependencyStatus["tool_approval_store"] = "backend:" + s.cfg.HostedTools.Approval.Backend
	}

	if s.handlers.chatHandler == nil {
		summary.RestartRequiredRoutes = append(summary.RestartRequiredRoutes, "chat")
	}
	if s.handlers.costHandler == nil {
		summary.RestartRequiredRoutes = append(summary.RestartRequiredRoutes, "cost")
	}
	if s.handlers.ragHandler == nil {
		summary.RestartRequiredRoutes = append(summary.RestartRequiredRoutes, "rag")
	}
	if s.handlers.multimodalHandler == nil {
		summary.RestartRequiredRoutes = append(summary.RestartRequiredRoutes, "multimodal")
	}

	sort.Strings(summary.EnabledCapabilities)
	sort.Strings(summary.DisabledCapabilities)
	sort.Strings(summary.RestartRequiredRoutes)
	return summary
}

func (s *Server) logStartupSummary() {
	if s == nil || s.logger == nil || s.cfg == nil {
		return
	}

	summary := s.startupSummary()
	s.logger.Info("Startup summary",
		zap.Strings("enabled_capabilities", summary.EnabledCapabilities),
		zap.Strings("disabled_capabilities", summary.DisabledCapabilities),
		zap.Any("dependency_status", summary.DependencyStatus),
		zap.Strings("restart_required_routes", summary.RestartRequiredRoutes),
		zap.Bool("hot_reload_enabled", summary.HotReloadEnabled),
		zap.String("metrics_bind_address", summary.MetricsBindAddress),
		zap.Bool("pprof_enabled", summary.PProfEnabled),
		zap.String("tool_approval_backend", summary.ToolApprovalBackend),
		zap.Bool("multimodal_enabled", summary.MultimodalEnabled),
		zap.String("multimodal_reference_backend", summary.MultimodalRefBackend),
	)
}

func requiredDependencyStatus(ready bool) string {
	if ready {
		return "required+ready"
	}
	return "required+missing"
}

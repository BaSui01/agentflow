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

	summary.HotReloadEnabled = s.configPath != "" && s.hotReloadManager != nil
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

	appendCapabilityState("chat", s.chatHandler != nil)
	appendCapabilityState("agent", s.agentHandler != nil)
	appendCapabilityState("health", s.healthHandler != nil)
	appendCapabilityState("protocol", s.protocolHandler != nil)
	appendCapabilityState("rag", s.ragHandler != nil)
	appendCapabilityState("workflow", s.workflowHandler != nil)
	appendCapabilityState("multimodal", s.multimodalHandler != nil)
	appendCapabilityState("cost", s.costHandler != nil)
	appendCapabilityState("api_key_management", s.apiKeyHandler != nil)
	appendCapabilityState("tool_registry", s.toolRegistryHandler != nil)
	appendCapabilityState("tool_provider_config", s.toolProviderHandler != nil)
	appendCapabilityState("tool_approval", s.toolApprovalHandler != nil)
	appendCapabilityState("mongo_runtime", s.mongoClient != nil)
	appendCapabilityState("audit", s.auditLogger != nil)
	appendCapabilityState("enhanced_memory", s.enhancedMemory != nil)
	appendCapabilityState("ab_testing", s.abTester != nil)

	summary.DependencyStatus["database"] = requiredDependencyStatus(s.db != nil)
	summary.DependencyStatus["mongodb"] = requiredDependencyStatus(s.mongoClient != nil)
	summary.DependencyStatus["llm_runtime"] = requiredDependencyStatus(s.provider != nil)

	if s.cfg.Multimodal.Enabled {
		summary.DependencyStatus["multimodal_redis"] = requiredDependencyStatus(s.multimodalRedis != nil)
	} else {
		summary.DependencyStatus["multimodal_redis"] = "disabled"
	}

	switch s.cfg.HostedTools.Approval.Backend {
	case "redis":
		summary.DependencyStatus["tool_approval_store"] = requiredDependencyStatus(s.toolApprovalRedis != nil)
	default:
		summary.DependencyStatus["tool_approval_store"] = "backend:" + s.cfg.HostedTools.Approval.Backend
	}

	if s.chatHandler == nil {
		summary.RestartRequiredRoutes = append(summary.RestartRequiredRoutes, "chat")
	}
	if s.costHandler == nil {
		summary.RestartRequiredRoutes = append(summary.RestartRequiredRoutes, "cost")
	}
	if s.ragHandler == nil {
		summary.RestartRequiredRoutes = append(summary.RestartRequiredRoutes, "rag")
	}
	if s.multimodalHandler == nil {
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

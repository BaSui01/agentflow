package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"go.uber.org/zap"
)

func TestCapabilityRegistry_RegisterAgent(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultRegistryConfig()
	config.EnableHealthCheck = false
	registry := NewCapabilityRegistry(config, logger)

	ctx := context.Background()

	// Create test agent
	card := a2a.NewAgentCard("test-agent", "Test Agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("code_review", "Review code", a2a.CapabilityTypeTask)
	card.AddCapability("code_analysis", "Analyze code", a2a.CapabilityTypeQuery)

	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
		Capabilities: []CapabilityInfo{
			{
				Capability: a2a.Capability{Name: "code_review", Description: "Review code", Type: a2a.CapabilityTypeTask},
				Status:     CapabilityStatusActive,
			},
			{
				Capability: a2a.Capability{Name: "code_analysis", Description: "Analyze code", Type: a2a.CapabilityTypeQuery},
				Status:     CapabilityStatusActive,
			},
		},
	}

	// Register agent
	err := registry.RegisterAgent(ctx, info)
	if err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	// Verify agent is registered
	retrieved, err := registry.GetAgent(ctx, "test-agent")
	if err != nil {
		t.Fatalf("failed to get agent: %v", err)
	}

	if retrieved.Card.Name != "test-agent" {
		t.Errorf("expected agent name 'test-agent', got '%s'", retrieved.Card.Name)
	}

	if len(retrieved.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(retrieved.Capabilities))
	}

	// Try to register same agent again (should fail)
	err = registry.RegisterAgent(ctx, info)
	if err == nil {
		t.Error("expected error when registering duplicate agent")
	}
}

func TestCapabilityRegistry_UnregisterAgent(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultRegistryConfig()
	config.EnableHealthCheck = false
	registry := NewCapabilityRegistry(config, logger)

	ctx := context.Background()

	// Create and register test agent
	card := a2a.NewAgentCard("test-agent", "Test Agent", "http://localhost:8080", "1.0.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
	}

	err := registry.RegisterAgent(ctx, info)
	if err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	// Unregister agent
	err = registry.UnregisterAgent(ctx, "test-agent")
	if err != nil {
		t.Fatalf("failed to unregister agent: %v", err)
	}

	// Verify agent is unregistered
	_, err = registry.GetAgent(ctx, "test-agent")
	if err == nil {
		t.Error("expected error when getting unregistered agent")
	}
}

func TestCapabilityRegistry_FindCapabilities(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultRegistryConfig()
	config.EnableHealthCheck = false
	registry := NewCapabilityRegistry(config, logger)

	ctx := context.Background()

	// Register multiple agents with same capability
	for i := 1; i <= 3; i++ {
		card := a2a.NewAgentCard(
			"agent-"+string(rune('0'+i)),
			"Test Agent",
			"http://localhost:808"+string(rune('0'+i)),
			"1.0.0",
		)
		info := &AgentInfo{
			Card:    card,
			Status:  AgentStatusOnline,
			IsLocal: true,
			Capabilities: []CapabilityInfo{
				{
					Capability: a2a.Capability{Name: "code_review", Description: "Review code", Type: a2a.CapabilityTypeTask},
					Status:     CapabilityStatusActive,
					Score:      float64(50 + i*10),
				},
			},
		}
		err := registry.RegisterAgent(ctx, info)
		if err != nil {
			t.Fatalf("failed to register agent %d: %v", i, err)
		}
	}

	// Find capabilities
	caps, err := registry.FindCapabilities(ctx, "code_review")
	if err != nil {
		t.Fatalf("failed to find capabilities: %v", err)
	}

	if len(caps) != 3 {
		t.Errorf("expected 3 capabilities, got %d", len(caps))
	}
}

func TestCapabilityRegistry_RecordExecution(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultRegistryConfig()
	config.EnableHealthCheck = false
	registry := NewCapabilityRegistry(config, logger)

	ctx := context.Background()

	// Register agent
	card := a2a.NewAgentCard("test-agent", "Test Agent", "http://localhost:8080", "1.0.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
		Capabilities: []CapabilityInfo{
			{
				Capability: a2a.Capability{Name: "code_review", Description: "Review code", Type: a2a.CapabilityTypeTask},
				Status:     CapabilityStatusActive,
				Score:      50.0,
			},
		},
	}
	err := registry.RegisterAgent(ctx, info)
	if err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	// Record successful executions
	for i := 0; i < 5; i++ {
		err = registry.RecordExecution(ctx, "test-agent", "code_review", true, 100*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to record execution: %v", err)
		}
	}

	// Record failed execution
	err = registry.RecordExecution(ctx, "test-agent", "code_review", false, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record execution: %v", err)
	}

	// Verify stats
	cap, err := registry.GetCapability(ctx, "test-agent", "code_review")
	if err != nil {
		t.Fatalf("failed to get capability: %v", err)
	}

	if cap.SuccessCount != 5 {
		t.Errorf("expected 5 successes, got %d", cap.SuccessCount)
	}

	if cap.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", cap.FailureCount)
	}

	// Score should be updated based on success rate (5/6 = 83.33%)
	expectedScore := (5.0 / 6.0) * 100
	if cap.Score < expectedScore-1 || cap.Score > expectedScore+1 {
		t.Errorf("expected score around %.2f, got %.2f", expectedScore, cap.Score)
	}
}

func TestCapabilityMatcher_Match(t *testing.T) {
	logger := zap.NewNop()
	regConfig := DefaultRegistryConfig()
	regConfig.EnableHealthCheck = false
	registry := NewCapabilityRegistry(regConfig, logger)

	matcherConfig := DefaultMatcherConfig()
	matcher := NewCapabilityMatcher(registry, matcherConfig, logger)

	ctx := context.Background()

	// Register agents with different capabilities
	agents := []struct {
		name         string
		capabilities []string
		score        float64
		load         float64
	}{
		{"agent-1", []string{"code_review", "code_analysis"}, 80.0, 0.2},
		{"agent-2", []string{"code_review", "testing"}, 90.0, 0.5},
		{"agent-3", []string{"documentation", "testing"}, 70.0, 0.1},
	}

	for _, a := range agents {
		card := a2a.NewAgentCard(a.name, "Test Agent", "http://localhost:8080", "1.0.0")
		caps := make([]CapabilityInfo, len(a.capabilities))
		for i, capName := range a.capabilities {
			caps[i] = CapabilityInfo{
				Capability: a2a.Capability{Name: capName, Description: capName, Type: a2a.CapabilityTypeTask},
				Status:     CapabilityStatusActive,
				Score:      a.score,
				Load:       a.load,
			}
		}
		info := &AgentInfo{
			Card:         card,
			Status:       AgentStatusOnline,
			IsLocal:      true,
			Capabilities: caps,
			Load:         a.load,
		}
		err := registry.RegisterAgent(ctx, info)
		if err != nil {
			t.Fatalf("failed to register agent %s: %v", a.name, err)
		}
	}

	// Test matching with required capabilities
	results, err := matcher.Match(ctx, &MatchRequest{
		RequiredCapabilities: []string{"code_review"},
		Strategy:             MatchStrategyBestMatch,
	})
	if err != nil {
		t.Fatalf("failed to match: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Test matching with least loaded strategy
	results, err = matcher.Match(ctx, &MatchRequest{
		RequiredCapabilities: []string{"code_review"},
		Strategy:             MatchStrategyLeastLoaded,
	})
	if err != nil {
		t.Fatalf("failed to match: %v", err)
	}

	if len(results) > 0 && results[0].Agent.Card.Name != "agent-1" {
		t.Errorf("expected agent-1 (least loaded), got %s", results[0].Agent.Card.Name)
	}
}

func TestCapabilityMatcher_MatchOne(t *testing.T) {
	logger := zap.NewNop()
	regConfig := DefaultRegistryConfig()
	regConfig.EnableHealthCheck = false
	registry := NewCapabilityRegistry(regConfig, logger)

	matcherConfig := DefaultMatcherConfig()
	matcher := NewCapabilityMatcher(registry, matcherConfig, logger)

	ctx := context.Background()

	// Register agent
	card := a2a.NewAgentCard("test-agent", "Test Agent", "http://localhost:8080", "1.0.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
		Capabilities: []CapabilityInfo{
			{
				Capability: a2a.Capability{Name: "code_review", Description: "Review code", Type: a2a.CapabilityTypeTask},
				Status:     CapabilityStatusActive,
				Score:      80.0,
			},
		},
	}
	err := registry.RegisterAgent(ctx, info)
	if err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	// Test MatchOne
	result, err := matcher.MatchOne(ctx, &MatchRequest{
		RequiredCapabilities: []string{"code_review"},
	})
	if err != nil {
		t.Fatalf("failed to match one: %v", err)
	}

	if result.Agent.Card.Name != "test-agent" {
		t.Errorf("expected test-agent, got %s", result.Agent.Card.Name)
	}

	// Test MatchOne with no matching capability
	_, err = matcher.MatchOne(ctx, &MatchRequest{
		RequiredCapabilities: []string{"nonexistent"},
	})
	if err == nil {
		t.Error("expected error when no matching agent found")
	}
}

func TestCapabilityComposer_Compose(t *testing.T) {
	logger := zap.NewNop()
	regConfig := DefaultRegistryConfig()
	regConfig.EnableHealthCheck = false
	registry := NewCapabilityRegistry(regConfig, logger)

	matcherConfig := DefaultMatcherConfig()
	matcher := NewCapabilityMatcher(registry, matcherConfig, logger)

	composerConfig := DefaultComposerConfig()
	composer := NewCapabilityComposer(registry, matcher, composerConfig, logger)

	ctx := context.Background()

	// Register agents with different capabilities
	agents := []struct {
		name         string
		capabilities []string
	}{
		{"agent-1", []string{"code_review"}},
		{"agent-2", []string{"testing"}},
		{"agent-3", []string{"documentation"}},
	}

	for _, a := range agents {
		card := a2a.NewAgentCard(a.name, "Test Agent", "http://localhost:8080", "1.0.0")
		caps := make([]CapabilityInfo, len(a.capabilities))
		for i, capName := range a.capabilities {
			caps[i] = CapabilityInfo{
				Capability: a2a.Capability{Name: capName, Description: capName, Type: a2a.CapabilityTypeTask},
				Status:     CapabilityStatusActive,
				Score:      80.0,
			}
		}
		info := &AgentInfo{
			Card:         card,
			Status:       AgentStatusOnline,
			IsLocal:      true,
			Capabilities: caps,
		}
		err := registry.RegisterAgent(ctx, info)
		if err != nil {
			t.Fatalf("failed to register agent %s: %v", a.name, err)
		}
	}

	// Test composition
	result, err := composer.Compose(ctx, &CompositionRequest{
		RequiredCapabilities: []string{"code_review", "testing"},
	})
	if err != nil {
		t.Fatalf("failed to compose: %v", err)
	}

	if !result.Complete {
		t.Error("expected complete composition")
	}

	if len(result.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(result.Agents))
	}

	if len(result.CapabilityMap) != 2 {
		t.Errorf("expected 2 capability mappings, got %d", len(result.CapabilityMap))
	}
}

func TestCapabilityComposer_DetectConflicts(t *testing.T) {
	logger := zap.NewNop()
	regConfig := DefaultRegistryConfig()
	regConfig.EnableHealthCheck = false
	registry := NewCapabilityRegistry(regConfig, logger)

	matcherConfig := DefaultMatcherConfig()
	matcher := NewCapabilityMatcher(registry, matcherConfig, logger)

	composerConfig := DefaultComposerConfig()
	composer := NewCapabilityComposer(registry, matcher, composerConfig, logger)

	// Register exclusive group
	composer.RegisterExclusiveGroup([]string{"gpu_compute", "cpu_compute"})

	ctx := context.Background()

	// Test conflict detection
	conflicts, err := composer.DetectConflicts(ctx, []string{"gpu_compute", "cpu_compute"})
	if err != nil {
		t.Fatalf("failed to detect conflicts: %v", err)
	}

	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflicts))
	}

	if conflicts[0].Type != ConflictTypeExclusive {
		t.Errorf("expected exclusive conflict, got %s", conflicts[0].Type)
	}
}

func TestCapabilityComposer_ResolveDependencies(t *testing.T) {
	logger := zap.NewNop()
	regConfig := DefaultRegistryConfig()
	regConfig.EnableHealthCheck = false
	registry := NewCapabilityRegistry(regConfig, logger)

	matcherConfig := DefaultMatcherConfig()
	matcher := NewCapabilityMatcher(registry, matcherConfig, logger)

	composerConfig := DefaultComposerConfig()
	composer := NewCapabilityComposer(registry, matcher, composerConfig, logger)

	// Register dependencies
	composer.RegisterDependency("testing", []string{"code_review"})
	composer.RegisterDependency("deployment", []string{"testing", "documentation"})

	ctx := context.Background()

	// Test dependency resolution
	deps, err := composer.ResolveDependencies(ctx, []string{"deployment"})
	if err != nil {
		t.Fatalf("failed to resolve dependencies: %v", err)
	}

	if len(deps) != 2 {
		t.Errorf("expected 2 dependency entries, got %d", len(deps))
	}

	// Check deployment dependencies
	deployDeps, ok := deps["deployment"]
	if !ok {
		t.Error("expected deployment dependencies")
	}

	if len(deployDeps) != 2 {
		t.Errorf("expected 2 deployment dependencies, got %d", len(deployDeps))
	}
}

func TestDiscoveryService_Integration(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultServiceConfig()
	config.Registry.EnableHealthCheck = false
	config.Protocol.EnableHTTP = false
	config.Protocol.EnableMulticast = false
	config.EnableAutoRegistration = false

	service := NewDiscoveryService(config, logger)

	ctx := context.Background()

	// Start service
	err := service.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start service: %v", err)
	}
	defer service.Stop(ctx)

	// Register agent
	card := a2a.NewAgentCard("test-agent", "Test Agent", "http://localhost:8080", "1.0.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
		Capabilities: []CapabilityInfo{
			{
				Capability: a2a.Capability{Name: "code_review", Description: "Review code", Type: a2a.CapabilityTypeTask},
				Status:     CapabilityStatusActive,
				Score:      80.0,
			},
		},
	}

	err = service.RegisterAgent(ctx, info)
	if err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	// Find agent
	agent, err := service.FindAgent(ctx, "review my code", []string{"code_review"})
	if err != nil {
		t.Fatalf("failed to find agent: %v", err)
	}

	if agent.Card.Name != "test-agent" {
		t.Errorf("expected test-agent, got %s", agent.Card.Name)
	}

	// Record execution
	err = service.RecordExecution(ctx, "test-agent", "code_review", true, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to record execution: %v", err)
	}

	// Unregister agent
	err = service.UnregisterAgent(ctx, "test-agent")
	if err != nil {
		t.Fatalf("failed to unregister agent: %v", err)
	}
}

func TestAgentInfoFromCard(t *testing.T) {
	card := a2a.NewAgentCard("test-agent", "Test Agent", "http://localhost:8080", "1.0.0")
	card.AddCapability("code_review", "Review code", a2a.CapabilityTypeTask)
	card.SetMetadata("language", "go")

	info := AgentInfoFromCard(card, true)

	if info.Card.Name != "test-agent" {
		t.Errorf("expected test-agent, got %s", info.Card.Name)
	}

	if !info.IsLocal {
		t.Error("expected IsLocal to be true")
	}

	if info.Status != AgentStatusOnline {
		t.Errorf("expected online status, got %s", info.Status)
	}

	if len(info.Capabilities) != 1 {
		t.Errorf("expected 1 capability, got %d", len(info.Capabilities))
	}
}

func TestCapabilityRegistry_EventSubscription(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultRegistryConfig()
	config.EnableHealthCheck = false
	registry := NewCapabilityRegistry(config, logger)

	ctx := context.Background()

	// Subscribe to events
	eventReceived := make(chan *DiscoveryEvent, 10)
	subID := registry.Subscribe(func(event *DiscoveryEvent) {
		eventReceived <- event
	})
	defer registry.Unsubscribe(subID)

	// Register agent
	card := a2a.NewAgentCard("test-agent", "Test Agent", "http://localhost:8080", "1.0.0")
	info := &AgentInfo{
		Card:    card,
		Status:  AgentStatusOnline,
		IsLocal: true,
	}

	err := registry.RegisterAgent(ctx, info)
	if err != nil {
		t.Fatalf("failed to register agent: %v", err)
	}

	// Wait for event
	select {
	case event := <-eventReceived:
		if event.Type != DiscoveryEventAgentRegistered {
			t.Errorf("expected agent_registered event, got %s", event.Type)
		}
		if event.AgentID != "test-agent" {
			t.Errorf("expected agent ID test-agent, got %s", event.AgentID)
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for event")
	}
}

// Package discovery provides Agent capability discovery and matching for multi-agent collaboration.
//
// The discovery package implements a comprehensive system for:
//   - Capability Registration: Agents can register their capabilities with the registry
//   - Capability Matching: Find the best agent for a given task based on capabilities
//   - Capability Composition: Combine capabilities from multiple agents for complex tasks
//   - Service Discovery: Discover agents via local, HTTP, or multicast protocols
//
// # Architecture
//
// The package consists of several key components:
//
//   - Registry: Stores and manages agent and capability information
//   - Matcher: Finds agents matching specific criteria using various strategies
//   - Composer: Creates compositions of capabilities from multiple agents
//   - Protocol: Handles service discovery via different protocols
//   - Service: Unified interface combining all components
//
// # Basic Usage
//
// Create and start a discovery service:
//
//	config := discovery.DefaultServiceConfig()
//	logger, _ := zap.NewProduction()
//	service := discovery.NewDiscoveryService(config, logger)
//
//	ctx := context.Background()
//	if err := service.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer service.Stop(ctx)
//
// Register an agent:
//
//	card := a2a.NewAgentCard("code-reviewer", "Code Review Agent", "http://localhost:8080", "1.0.0")
//	card.AddCapability("code_review", "Review code quality", a2a.CapabilityTypeTask)
//
//	info := discovery.AgentInfoFromCard(card, true)
//	if err := service.RegisterAgent(ctx, info); err != nil {
//	    log.Fatal(err)
//	}
//
// Find an agent for a task:
//
//	agent, err := service.FindAgent(ctx, "review my Go code", []string{"code_review"})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Found agent: %s\n", agent.Card.Name)
//
// # Matching Strategies
//
// The matcher supports several strategies:
//
//   - BestMatch: Returns the best matching agent based on overall score
//   - LeastLoaded: Returns the least loaded matching agent
//   - HighestScore: Returns the agent with the highest capability score
//   - RoundRobin: Returns agents in round-robin order
//   - Random: Returns a random matching agent
//
// Example with specific strategy:
//
//	results, err := service.FindAgents(ctx, &discovery.MatchRequest{
//	    TaskDescription:      "analyze code for security issues",
//	    RequiredCapabilities: []string{"security_analysis"},
//	    Strategy:             discovery.MatchStrategyLeastLoaded,
//	    MaxLoad:              0.8,
//	    Limit:                5,
//	})
//
// # Capability Composition
//
// For complex tasks requiring multiple capabilities:
//
//	result, err := service.ComposeCapabilities(ctx, &discovery.CompositionRequest{
//	    TaskDescription:      "full code review pipeline",
//	    RequiredCapabilities: []string{"code_review", "security_analysis", "testing"},
//	    AllowPartial:         false,
//	})
//
//	if result.Complete {
//	    for cap, agentID := range result.CapabilityMap {
//	        fmt.Printf("Capability %s -> Agent %s\n", cap, agentID)
//	    }
//	}
//
// # Health Checking
//
// The registry includes automatic health checking:
//
//	config := discovery.DefaultRegistryConfig()
//	config.EnableHealthCheck = true
//	config.HealthCheckInterval = 30 * time.Second
//	config.UnhealthyThreshold = 3
//
// # Event Subscription
//
// Subscribe to discovery events:
//
//	subID := service.Subscribe(func(event *discovery.DiscoveryEvent) {
//	    switch event.Type {
//	    case discovery.DiscoveryEventAgentRegistered:
//	        fmt.Printf("Agent registered: %s\n", event.AgentID)
//	    case discovery.DiscoveryEventHealthCheckFailed:
//	        fmt.Printf("Agent unhealthy: %s\n", event.AgentID)
//	    }
//	})
//	defer service.Unsubscribe(subID)
//
// # Integration with Agents
//
// Use the AgentDiscoveryIntegration for seamless integration:
//
//	integration := discovery.NewAgentDiscoveryIntegration(service, nil, logger)
//	integration.Start(ctx)
//	defer integration.Stop(ctx)
//
//	// Register an agent that implements AgentCapabilityProvider
//	integration.RegisterAgent(ctx, myAgent)
//
//	// Set load reporter for automatic load updates
//	integration.SetLoadReporter(myAgent.ID(), func() float64 {
//	    return calculateCurrentLoad()
//	})
//
//	// Record execution results for capability scoring
//	integration.RecordExecution(ctx, myAgent.ID(), "code_review", true, 100*time.Millisecond)
package discovery

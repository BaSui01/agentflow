package bootstrap

import (
	"context"
	"fmt"

	agentmemory "github.com/BaSui01/agentflow/agent/capabilities/memory"
	agenttools "github.com/BaSui01/agentflow/agent/capabilities/tools"
	"github.com/BaSui01/agentflow/agent/observability/evaluation"
	mongostore "github.com/BaSui01/agentflow/agent/persistence/mongodb"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"go.uber.org/zap"
)

// MongoRuntimeWiring contains optional runtime capabilities wired from MongoDB stores.
type MongoRuntimeWiring struct {
	AuditLogger    *llmtools.DefaultAuditLogger
	EnhancedMemory *agentmemory.EnhancedMemorySystem
	ABTester       *evaluation.ABTester
}

// WireMongoRuntimeStores wires resolver/discovery stores and returns optional runtime capabilities.
func WireMongoRuntimeStores(
	ctx context.Context,
	client *mongoclient.Client,
	resolver *agent.CachingResolver,
	discoveryRegistry *agenttools.CapabilityRegistry,
	logger *zap.Logger,
) (*MongoRuntimeWiring, error) {
	promptStore, err := mongostore.NewPromptStore(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB prompt store: %w", err)
	}
	resolver.WithPromptStore(mongostore.NewPromptStoreAdapter(promptStore))
	logger.Info("MongoDB prompt store initialized")

	convStore, err := mongostore.NewConversationStore(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB conversation store: %w", err)
	}
	resolver.WithConversationStore(mongostore.NewConversationStoreAdapter(convStore))
	logger.Info("MongoDB conversation store initialized")

	runStore, err := mongostore.NewRunStore(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB run store: %w", err)
	}
	resolver.WithRunStore(mongostore.NewRunStoreAdapter(runStore))
	logger.Info("MongoDB run store initialized")

	auditBackend, err := mongostore.NewAuditBackend(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to create MongoDB audit backend: %w", err)
	}
	auditLogger := llmtools.NewAuditLogger(&llmtools.AuditLoggerConfig{
		Backends: []llmtools.AuditBackend{auditBackend},
	}, logger)
	logger.Info("MongoDB audit backend initialized")

	memoryStore, err := mongostore.NewMemoryStore(ctx, client)
	if err != nil {
		logger.Warn("failed to create MongoDB memory store, enhanced memory disabled", zap.Error(err))
	}

	var episodicStore *mongostore.MongoEpisodicStore
	if memoryStore != nil {
		episodicStore, err = mongostore.NewEpisodicStore(ctx, client)
		if err != nil {
			logger.Warn("failed to create MongoDB episodic store", zap.Error(err))
		}
	}

	var knowledgeGraph *mongostore.MongoKnowledgeGraph
	if memoryStore != nil {
		knowledgeGraph, err = mongostore.NewKnowledgeGraph(ctx, client)
		if err != nil {
			logger.Warn("failed to create MongoDB knowledge graph", zap.Error(err))
		}
	}

	var observationStore *mongostore.MongoObservationStore
	if memoryStore != nil {
		observationStore, err = mongostore.NewObservationStore(ctx, client)
		if err != nil {
			logger.Warn("failed to create MongoDB observation store", zap.Error(err))
		}
	}

	var enhancedMemory *agentmemory.EnhancedMemorySystem
	if memoryStore != nil {
		memCfg := agentmemory.DefaultEnhancedMemoryConfig()
		memCfg.EpisodicEnabled = episodicStore != nil
		memCfg.SemanticEnabled = knowledgeGraph != nil
		memCfg.ObservationEnabled = observationStore != nil

		working := agentmemory.NewInMemoryMemoryStore(agentmemory.InMemoryMemoryStoreConfig{
			MaxEntries: memCfg.WorkingMemorySize,
		}, logger)

		var episodic agentmemory.EpisodicStore
		if episodicStore != nil {
			episodic = episodicStore
		}
		var semantic agentmemory.KnowledgeGraph
		if knowledgeGraph != nil {
			semantic = knowledgeGraph
		}
		var obsStore agentmemory.ObservationStore
		if observationStore != nil {
			obsStore = observationStore
		}

		enhancedMemory = agentmemory.NewEnhancedMemorySystem(
			memoryStore, working, nil, episodic, semantic, obsStore, memCfg, logger,
		)
		resolver.WithEnhancedMemory(enhancedMemory)
		logger.Info("MongoDB enhanced memory system initialized",
			zap.Bool("episodic", episodicStore != nil),
			zap.Bool("semantic", knowledgeGraph != nil),
			zap.Bool("observation", observationStore != nil),
		)
	}

	var abTester *evaluation.ABTester
	expStore, err := mongostore.NewExperimentStore(ctx, client)
	if err != nil {
		logger.Warn("failed to create MongoDB experiment store, A/B testing disabled", zap.Error(err))
	} else {
		abTester = evaluation.NewABTester(expStore, logger)
		logger.Info("MongoDB experiment store initialized")
	}

	regStore, err := mongostore.NewRegistryStore(ctx, client)
	if err != nil {
		logger.Warn("failed to create MongoDB registry store, discovery persistence disabled", zap.Error(err))
	} else {
		discoveryRegistry.SetStore(regStore)
		logger.Info("MongoDB registry store initialized")
	}

	return &MongoRuntimeWiring{
		AuditLogger:    auditLogger,
		EnhancedMemory: enhancedMemory,
		ABTester:       abTester,
	}, nil
}

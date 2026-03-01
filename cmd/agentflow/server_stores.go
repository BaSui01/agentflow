package main

import (
	"context"
	"fmt"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/agent/evaluation"
	"github.com/BaSui01/agentflow/agent/memory"
	mongostore "github.com/BaSui01/agentflow/agent/persistence/mongodb"
	"github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/agent/skills"
	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"go.uber.org/zap"
)

func (s *Server) initMongoDB() error {
	client, err := mongoclient.NewClient(s.cfg.MongoDB, s.logger)
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	s.mongoClient = client
	s.logger.Info("MongoDB client initialized",
		zap.String("database", s.cfg.MongoDB.Database),
	)
	return nil
}

func (s *Server) wireMongoStores(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	promptStore, err := mongostore.NewPromptStore(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB prompt store: %w", err)
	}
	resolver.WithPromptStore(mongostore.NewPromptStoreAdapter(promptStore))
	s.logger.Info("MongoDB prompt store initialized")

	convStore, err := mongostore.NewConversationStore(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB conversation store: %w", err)
	}
	resolver.WithConversationStore(mongostore.NewConversationStoreAdapter(convStore))
	s.logger.Info("MongoDB conversation store initialized")

	runStore, err := mongostore.NewRunStore(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB run store: %w", err)
	}
	resolver.WithRunStore(mongostore.NewRunStoreAdapter(runStore))
	s.logger.Info("MongoDB run store initialized")

	auditBackend, err := mongostore.NewAuditBackend(ctx, s.mongoClient)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB audit backend: %w", err)
	}
	s.auditLogger = tools.NewAuditLogger(&tools.AuditLoggerConfig{
		Backends: []tools.AuditBackend{auditBackend},
	}, s.logger)
	s.logger.Info("MongoDB audit backend initialized")

	memoryStore, err := mongostore.NewMemoryStore(ctx, s.mongoClient)
	if err != nil {
		s.logger.Warn("failed to create MongoDB memory store, enhanced memory disabled", zap.Error(err))
	}

	var episodicStore *mongostore.MongoEpisodicStore
	if memoryStore != nil {
		episodicStore, err = mongostore.NewEpisodicStore(ctx, s.mongoClient)
		if err != nil {
			s.logger.Warn("failed to create MongoDB episodic store", zap.Error(err))
		}
	}

	var knowledgeGraph *mongostore.MongoKnowledgeGraph
	if memoryStore != nil {
		knowledgeGraph, err = mongostore.NewKnowledgeGraph(ctx, s.mongoClient)
		if err != nil {
			s.logger.Warn("failed to create MongoDB knowledge graph", zap.Error(err))
		}
	}

	if memoryStore != nil {
		memCfg := memory.DefaultEnhancedMemoryConfig()
		memCfg.EpisodicEnabled = episodicStore != nil
		memCfg.SemanticEnabled = knowledgeGraph != nil

		working := memory.NewInMemoryMemoryStore(memory.InMemoryMemoryStoreConfig{
			MaxEntries: memCfg.WorkingMemorySize,
		}, s.logger)

		var episodic memory.EpisodicStore
		if episodicStore != nil {
			episodic = episodicStore
		}
		var semantic memory.KnowledgeGraph
		if knowledgeGraph != nil {
			semantic = knowledgeGraph
		}

		s.enhancedMemory = memory.NewEnhancedMemorySystem(
			memoryStore, working, nil, episodic, semantic, memCfg, s.logger,
		)
		s.logger.Info("MongoDB enhanced memory system initialized",
			zap.Bool("episodic", episodicStore != nil),
			zap.Bool("semantic", knowledgeGraph != nil),
		)
	}

	expStore, err := mongostore.NewExperimentStore(ctx, s.mongoClient)
	if err != nil {
		s.logger.Warn("failed to create MongoDB experiment store, A/B testing disabled", zap.Error(err))
	} else {
		s.abTester = evaluation.NewABTester(expStore, s.logger)
		s.logger.Info("MongoDB experiment store initialized")
	}

	regStore, err := mongostore.NewRegistryStore(ctx, s.mongoClient)
	if err != nil {
		s.logger.Warn("failed to create MongoDB registry store, discovery persistence disabled", zap.Error(err))
	} else {
		discoveryRegistry.SetStore(regStore)
		s.logger.Info("MongoDB registry store initialized")
	}

	return nil
}

func (s *Server) wireDefaultRuntimeAgent(agentRegistry *agent.AgentRegistry) {
	if s.provider == nil {
		return
	}

	agentRegistry.Register("default", func(
		cfg agent.Config,
		provider llm.Provider,
		mem agent.MemoryManager,
		tm agent.ToolManager,
		bus agent.EventBus,
		logger *zap.Logger,
	) (agent.Agent, error) {
		opts := runtime.DefaultBuildOptions()
		opts.EnableAll = false
		opts.EnableSkills = true
		opts.SkillsConfig = &skills.SkillManagerConfig{MaxLoadedSkills: 50}
		opts.InitAgent = true
		return runtime.BuildAgent(context.Background(), cfg, provider, logger, opts)
	})

	s.logger.Info("Default runtime agent factory registered")
}

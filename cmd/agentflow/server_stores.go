package main

import (
	"context"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/discovery"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
)

func (s *Server) initMongoDB() error {
	client, err := bootstrap.BuildMongoClient(s.cfg.MongoDB, s.logger)
	if err != nil {
		return err
	}

	s.mongoClient = client
	return nil
}

func (s *Server) wireMongoStores(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wiring, err := bootstrap.WireMongoRuntimeStores(ctx, s.mongoClient, resolver, discoveryRegistry, s.logger)
	if err != nil {
		return err
	}

	s.auditLogger = wiring.AuditLogger
	s.enhancedMemory = wiring.EnhancedMemory
	s.abTester = wiring.ABTester

	return nil
}

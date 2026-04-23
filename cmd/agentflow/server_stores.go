package main

import (
	"context"
	"time"

	discovery "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
)

func (s *Server) initMongoDB() error {
	client, err := bootstrap.BuildMongoClient(s.cfg.MongoDB, s.logger)
	if err != nil {
		return err
	}

	s.infra.mongoClient = client
	return nil
}

func (s *Server) wireMongoStores(resolver *agent.CachingResolver, discoveryRegistry *discovery.CapabilityRegistry) error {
	if s.infra.mongoClient == nil || resolver == nil || discoveryRegistry == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wiring, err := bootstrap.WireMongoRuntimeStores(ctx, s.infra.mongoClient, resolver, discoveryRegistry, s.logger)
	if err != nil {
		return err
	}

	s.infra.auditLogger = wiring.AuditLogger
	s.infra.enhancedMemory = wiring.EnhancedMemory
	s.infra.abTester = wiring.ABTester

	return nil
}

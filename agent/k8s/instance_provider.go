package k8s

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// InMemoryInstanceProvider manages agent instances in memory.
// It implements InstanceProvider for testing and local development.
type InMemoryInstanceProvider struct {
	instances map[string]*AgentInstance
	logger    *zap.Logger
	mu        sync.RWMutex
}

// NewInMemoryInstanceProvider creates a new in-memory instance provider.
func NewInMemoryInstanceProvider(logger *zap.Logger) *InMemoryInstanceProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &InMemoryInstanceProvider{
		instances: make(map[string]*AgentInstance),
		logger:    logger,
	}
}

func generateInstanceID(namespace, name string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%s-%d", namespace, name, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s-%s", namespace, name, hex.EncodeToString(b))
}

// CreateInstance creates a new agent instance in memory.
func (p *InMemoryInstanceProvider) CreateInstance(_ context.Context, agent *AgentCRD) (*AgentInstance, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	inst := &AgentInstance{
		ID:        generateInstanceID(agent.Metadata.Namespace, agent.Metadata.Name),
		AgentName: agent.Metadata.Name,
		Namespace: agent.Metadata.Namespace,
		Status:    InstanceStatusPending,
		StartTime: time.Now(),
		Labels:    agent.Metadata.Labels,
	}

	p.instances[inst.ID] = inst
	p.logger.Debug("instance created", zap.String("id", inst.ID))
	return inst, nil
}

// DeleteInstance removes an instance by ID.
func (p *InMemoryInstanceProvider) DeleteInstance(_ context.Context, instanceID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, ok := p.instances[instanceID]; !ok {
		return fmt.Errorf("instance not found: %s", instanceID)
	}
	delete(p.instances, instanceID)
	p.logger.Debug("instance deleted", zap.String("id", instanceID))
	return nil
}

// GetInstanceStatus returns the status of an instance.
func (p *InMemoryInstanceProvider) GetInstanceStatus(_ context.Context, instanceID string) (InstanceStatus, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	inst, ok := p.instances[instanceID]
	if !ok {
		return "", fmt.Errorf("instance not found: %s", instanceID)
	}
	return inst.Status, nil
}

// ListInstances returns all instances for a given namespace and agent name.
func (p *InMemoryInstanceProvider) ListInstances(_ context.Context, namespace, name string) ([]*AgentInstance, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var result []*AgentInstance
	for _, inst := range p.instances {
		if inst.AgentName == name && inst.Namespace == namespace {
			result = append(result, inst)
		}
	}
	return result, nil
}


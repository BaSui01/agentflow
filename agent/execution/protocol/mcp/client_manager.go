package mcp

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// TransportFactory creates a new Transport for reconnection.
type TransportFactory func() (Transport, error)

type clientEntry struct {
	client           *DefaultMCPClient
	transportFactory TransportFactory
	failCount        int
}

type reconnectCandidate struct {
	name             string
	client           *DefaultMCPClient
	transportFactory TransportFactory
	failCount        int
}

// MCPClientManager manages multiple MCP server connections by name,
// with health checking and automatic reconnection support.
type MCPClientManager struct {
	clients map[string]*clientEntry
	mu      sync.RWMutex
	logger  *zap.Logger

	healthDone     chan struct{}
	healthStop     chan struct{}
	healthOnce     sync.Once
	healthStopOnce sync.Once
	healthRunning  atomic.Bool
}

// NewMCPClientManager creates a new multi-server client manager.
func NewMCPClientManager(logger *zap.Logger) *MCPClientManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MCPClientManager{
		clients:    make(map[string]*clientEntry),
		logger:     logger.With(zap.String("component", "mcp_client_manager")),
		healthDone: make(chan struct{}),
		healthStop: make(chan struct{}),
	}
}

// Register adds a named MCP client and initializes it.
// On success, the manager takes ownership of the transport.
// On failure, the caller is responsible for closing the transport.
func (m *MCPClientManager) Register(ctx context.Context, name string, transport Transport) error {
	return m.RegisterWithFactory(ctx, name, transport, nil)
}

// RegisterWithFactory adds a named MCP client with an optional factory
// for automatic reconnection.
func (m *MCPClientManager) RegisterWithFactory(ctx context.Context, name string, transport Transport, factory TransportFactory) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.clients[name]; exists {
		return fmt.Errorf("mcp server %q already registered", name)
	}

	client := NewDefaultMCPClient(transport, m.logger)
	if err := client.Initialize(ctx); err != nil {
		_ = client.Close()
		return fmt.Errorf("initialize mcp server %q: %w", name, err)
	}

	m.clients[name] = &clientEntry{
		client:           client,
		transportFactory: factory,
	}
	m.logger.Info("registered mcp server", zap.String("name", name))
	return nil
}

// Get returns the client for the named server.
func (m *MCPClientManager) Get(name string) (*DefaultMCPClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.clients[name]
	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", name)
	}
	return entry.client, nil
}

// Remove closes and removes the named server connection.
func (m *MCPClientManager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.clients[name]
	if !ok {
		return fmt.Errorf("mcp server %q not found", name)
	}
	err := entry.client.Close()
	delete(m.clients, name)
	m.logger.Info("removed mcp server", zap.String("name", name))
	return err
}

// ListServers returns all registered server names.
func (m *MCPClientManager) ListServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.clients))
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// ListAllTools returns tools from all registered servers.
func (m *MCPClientManager) ListAllTools(ctx context.Context) (map[string][]MCPTool, error) {
	m.mu.RLock()
	snapshot := make(map[string]*DefaultMCPClient, len(m.clients))
	for name, entry := range m.clients {
		snapshot[name] = entry.client
	}
	m.mu.RUnlock()

	result := make(map[string][]MCPTool, len(snapshot))
	for name, client := range snapshot {
		tools, err := client.ListTools(ctx)
		if err != nil {
			m.logger.Warn("failed to list tools from server", zap.String("server", name), zap.Error(err))
			continue
		}
		result[name] = tools
	}
	return result, nil
}

// StartHealthCheck launches a background goroutine that periodically checks
// all client transports and attempts reconnection for unhealthy ones.
func (m *MCPClientManager) StartHealthCheck(ctx context.Context, interval time.Duration) {
	m.healthOnce.Do(func() {
		m.healthRunning.Store(true)
		go m.healthLoop(ctx, interval)
	})
}

func (m *MCPClientManager) healthLoop(ctx context.Context, interval time.Duration) {
	defer close(m.healthDone)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.healthStop:
			return
		case <-ticker.C:
			m.checkAndReconnect(ctx)
		}
	}
}

const maxReconnectBackoff = 5 * time.Minute

func (m *MCPClientManager) checkAndReconnect(ctx context.Context) {
	for _, candidate := range m.snapshotReconnectCandidates() {
		m.logger.Warn("mcp transport dead, attempting reconnect",
			zap.String("server", candidate.name),
			zap.Int("fail_count", candidate.failCount))

		if candidate.transportFactory == nil {
			m.logger.Warn("no transport factory for server, cannot reconnect",
				zap.String("server", candidate.name))
			continue
		}

		if err := waitReconnectBackoff(ctx, calculateReconnectBackoff(candidate.failCount)); err != nil {
			return
		}

		newTransport, err := candidate.transportFactory()
		if err != nil {
			m.incrementFailCount(candidate.name, candidate.client)
			m.logger.Error("reconnect transport creation failed",
				zap.String("server", candidate.name), zap.Error(err))
			continue
		}

		newClient := NewDefaultMCPClient(newTransport, m.logger)
		if err := newClient.Initialize(ctx); err != nil {
			m.incrementFailCount(candidate.name, candidate.client)
			_ = newClient.Close()
			m.logger.Error("reconnect initialization failed",
				zap.String("server", candidate.name), zap.Error(err))
			continue
		}

		if !m.swapReconnectedClient(candidate.name, candidate.client, newClient) {
			_ = newClient.Close()
			continue
		}

		if candidate.client != nil {
			_ = candidate.client.Close()
		}
		m.logger.Info("reconnected mcp server", zap.String("server", candidate.name))
	}
}

func (m *MCPClientManager) snapshotReconnectCandidates() []reconnectCandidate {
	m.mu.Lock()
	defer m.mu.Unlock()

	candidates := make([]reconnectCandidate, 0, len(m.clients))
	for name, entry := range m.clients {
		if entry == nil {
			continue
		}
		if entry.client != nil && entry.client.transport != nil && entry.client.transport.IsAlive() {
			entry.failCount = 0
			continue
		}
		candidates = append(candidates, reconnectCandidate{
			name:             name,
			client:           entry.client,
			transportFactory: entry.transportFactory,
			failCount:        entry.failCount,
		})
	}
	return candidates
}

func calculateReconnectBackoff(failCount int) time.Duration {
	if failCount < 0 {
		return 0
	}
	return time.Duration(math.Min(
		float64(time.Second)*math.Pow(2, float64(failCount)),
		float64(maxReconnectBackoff),
	))
}

func waitReconnectBackoff(ctx context.Context, backoff time.Duration) error {
	if backoff <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(backoff)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *MCPClientManager) incrementFailCount(name string, current *DefaultMCPClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.clients[name]
	if !ok || entry == nil || entry.client != current {
		return
	}
	entry.failCount++
}

func (m *MCPClientManager) swapReconnectedClient(name string, current *DefaultMCPClient, next *DefaultMCPClient) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.clients[name]
	if !ok || entry == nil || entry.client != current {
		return false
	}
	entry.client = next
	entry.failCount = 0
	return true
}

// CloseAll shuts down all client connections and stops the health checker.
func (m *MCPClientManager) CloseAll() error {
	m.healthStopOnce.Do(func() { close(m.healthStop) })
	if m.healthRunning.Load() {
		<-m.healthDone
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, entry := range m.clients {
		if err := entry.client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		m.logger.Debug("closed mcp server", zap.String("name", name))
	}
	m.clients = make(map[string]*clientEntry)
	return firstErr
}

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
		panic("agent.MCPClientManager: logger is required and cannot be nil")
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
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, entry := range m.clients {
		if entry.client.transport != nil && entry.client.transport.IsAlive() {
			entry.failCount = 0
			continue
		}

		m.logger.Warn("mcp transport dead, attempting reconnect",
			zap.String("server", name),
			zap.Int("fail_count", entry.failCount))

		if entry.transportFactory == nil {
			m.logger.Warn("no transport factory for server, cannot reconnect",
				zap.String("server", name))
			continue
		}

		backoff := time.Duration(math.Min(
			float64(time.Second)*math.Pow(2, float64(entry.failCount)),
			float64(maxReconnectBackoff),
		))
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		newTransport, err := entry.transportFactory()
		if err != nil {
			entry.failCount++
			m.logger.Error("reconnect transport creation failed",
				zap.String("server", name), zap.Error(err))
			continue
		}

		_ = entry.client.Close()
		newClient := NewDefaultMCPClient(newTransport, m.logger)
		if err := newClient.Initialize(ctx); err != nil {
			entry.failCount++
			_ = newClient.Close()
			m.logger.Error("reconnect initialization failed",
				zap.String("server", name), zap.Error(err))
			continue
		}

		entry.client = newClient
		entry.failCount = 0
		m.logger.Info("reconnected mcp server", zap.String("server", name))
	}
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

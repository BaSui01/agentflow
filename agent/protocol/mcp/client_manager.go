package mcp

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// MCPClientManager manages multiple MCP server connections by name.
type MCPClientManager struct {
	clients map[string]*DefaultMCPClient
	mu      sync.RWMutex
	logger  *zap.Logger
}

// NewMCPClientManager creates a new multi-server client manager.
func NewMCPClientManager(logger *zap.Logger) *MCPClientManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MCPClientManager{
		clients: make(map[string]*DefaultMCPClient),
		logger:  logger.With(zap.String("component", "mcp_client_manager")),
	}
}

// Register adds a named MCP client and initializes it.
// On success, the manager takes ownership of the transport.
// On failure, the caller is responsible for closing the transport.
func (m *MCPClientManager) Register(ctx context.Context, name string, transport Transport) error {
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

	m.clients[name] = client
	m.logger.Info("registered mcp server", zap.String("name", name))
	return nil
}

// Get returns the client for the named server.
func (m *MCPClientManager) Get(name string) (*DefaultMCPClient, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.clients[name]
	if !ok {
		return nil, fmt.Errorf("mcp server %q not found", name)
	}
	return c, nil
}

// Remove closes and removes the named server connection.
func (m *MCPClientManager) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.clients[name]
	if !ok {
		return fmt.Errorf("mcp server %q not found", name)
	}
	err := c.Close()
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
	for name, client := range m.clients {
		snapshot[name] = client
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

// CloseAll shuts down all client connections.
func (m *MCPClientManager) CloseAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, client := range m.clients {
		if err := client.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		m.logger.Debug("closed mcp server", zap.String("name", name))
	}
	m.clients = make(map[string]*DefaultMCPClient)
	return firstErr
}

package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DiscoveryProtocol is the default implementation of the Protocol interface.
// It supports local (in-process), HTTP, and multicast discovery.
type DiscoveryProtocol struct {
	config   *ProtocolConfig
	registry Registry
	logger   *zap.Logger

	// Local discovery
	localAgents map[string]*AgentInfo
	localMu     sync.RWMutex

	// HTTP server for remote discovery
	httpServer *http.Server
	httpMux    *http.ServeMux

	// Multicast discovery
	multicastConn *net.UDPConn
	multicastAddr *net.UDPAddr

	// Event handlers
	handlers   map[string]func(*AgentInfo)
	handlerMu  sync.RWMutex
	handlerSeq int

	// State
	running bool
	done    chan struct{}
	wg      sync.WaitGroup
}

// ProtocolConfig holds configuration for the discovery protocol.
type ProtocolConfig struct {
	// EnableLocal enables local (in-process) discovery.
	EnableLocal bool `json:"enable_local"`

	// EnableHTTP enables HTTP-based remote discovery.
	EnableHTTP bool `json:"enable_http"`

	// HTTPPort is the port for HTTP discovery server.
	HTTPPort int `json:"http_port"`

	// HTTPHost is the host for HTTP discovery server.
	HTTPHost string `json:"http_host"`

	// EnableMulticast enables multicast-based discovery.
	EnableMulticast bool `json:"enable_multicast"`

	// MulticastAddress is the multicast group address.
	MulticastAddress string `json:"multicast_address"`

	// MulticastPort is the multicast port.
	MulticastPort int `json:"multicast_port"`

	// AnnounceInterval is the interval for periodic announcements.
	AnnounceInterval time.Duration `json:"announce_interval"`

	// DiscoveryTimeout is the timeout for discovery operations.
	DiscoveryTimeout time.Duration `json:"discovery_timeout"`

	// MaxPeers is the maximum number of peers to track.
	MaxPeers int `json:"max_peers"`
}

// DefaultProtocolConfig returns a ProtocolConfig with sensible defaults.
func DefaultProtocolConfig() *ProtocolConfig {
	return &ProtocolConfig{
		EnableLocal:      true,
		EnableHTTP:       true,
		HTTPPort:         8765,
		HTTPHost:         "0.0.0.0",
		EnableMulticast:  false,
		MulticastAddress: "239.255.255.250",
		MulticastPort:    1900,
		AnnounceInterval: 30 * time.Second,
		DiscoveryTimeout: 5 * time.Second,
		MaxPeers:         100,
	}
}

// NewDiscoveryProtocol creates a new discovery protocol.
func NewDiscoveryProtocol(config *ProtocolConfig, registry Registry, logger *zap.Logger) *DiscoveryProtocol {
	if config == nil {
		config = DefaultProtocolConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &DiscoveryProtocol{
		config:      config,
		registry:    registry,
		logger:      logger.With(zap.String("component", "discovery_protocol")),
		localAgents: make(map[string]*AgentInfo),
		handlers:    make(map[string]func(*AgentInfo)),
		done:        make(chan struct{}),
	}
}

// Start starts the discovery protocol.
func (p *DiscoveryProtocol) Start(ctx context.Context) error {
	if p.running {
		return fmt.Errorf("protocol already running")
	}

	// Start HTTP server if enabled
	if p.config.EnableHTTP {
		if err := p.startHTTPServer(); err != nil {
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
	}

	// Start multicast listener if enabled
	if p.config.EnableMulticast {
		if err := p.startMulticast(); err != nil {
			p.logger.Warn("failed to start multicast", zap.Error(err))
			// Don't fail if multicast fails
		}
	}

	p.running = true
	p.logger.Info("discovery protocol started",
		zap.Bool("http", p.config.EnableHTTP),
		zap.Bool("multicast", p.config.EnableMulticast),
	)

	return nil
}

// Stop stops the discovery protocol.
func (p *DiscoveryProtocol) Stop(ctx context.Context) error {
	if !p.running {
		return nil
	}

	close(p.done)

	// Stop HTTP server
	if p.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := p.httpServer.Shutdown(shutdownCtx); err != nil {
			p.logger.Error("failed to shutdown HTTP server", zap.Error(err))
		}
	}

	// Stop multicast
	if p.multicastConn != nil {
		p.multicastConn.Close()
	}

	p.wg.Wait()
	p.running = false
	p.logger.Info("discovery protocol stopped")

	return nil
}

// Announce announces the local agent to the network.
func (p *DiscoveryProtocol) Announce(ctx context.Context, info *AgentInfo) error {
	if info == nil || info.Card == nil {
		return fmt.Errorf("invalid agent info")
	}

	agentID := info.Card.Name

	// Register locally
	if p.config.EnableLocal {
		p.localMu.Lock()
		p.localAgents[agentID] = info
		p.localMu.Unlock()
	}

	// Register with registry
	if p.registry != nil {
		if err := p.registry.RegisterAgent(ctx, info); err != nil {
			// Try update if already registered
			if updateErr := p.registry.UpdateAgent(ctx, info); updateErr != nil {
				return fmt.Errorf("failed to register/update agent: %w", err)
			}
		}
	}

	// Announce via multicast if enabled
	if p.config.EnableMulticast && p.multicastConn != nil {
		if err := p.announceMulticast(info); err != nil {
			p.logger.Warn("failed to announce via multicast", zap.Error(err))
		}
	}

	p.logger.Debug("agent announced", zap.String("agent_id", agentID))

	// Notify handlers
	p.notifyHandlers(info)

	return nil
}

// Discover discovers agents on the network.
func (p *DiscoveryProtocol) Discover(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	agents := make([]*AgentInfo, 0)
	seen := make(map[string]bool)

	// Discover local agents
	if p.config.EnableLocal {
		localAgents := p.discoverLocal(filter)
		for _, agent := range localAgents {
			if !seen[agent.Card.Name] {
				agents = append(agents, agent)
				seen[agent.Card.Name] = true
			}
		}
	}

	// Discover from registry
	if p.registry != nil {
		registryAgents, err := p.registry.ListAgents(ctx)
		if err != nil {
			p.logger.Warn("failed to list agents from registry", zap.Error(err))
		} else {
			for _, agent := range registryAgents {
				if !seen[agent.Card.Name] && p.matchesFilter(agent, filter) {
					agents = append(agents, agent)
					seen[agent.Card.Name] = true
				}
			}
		}
	}

	// Discover via multicast if enabled
	if p.config.EnableMulticast {
		multicastAgents, err := p.discoverMulticast(ctx, filter)
		if err != nil {
			p.logger.Warn("failed to discover via multicast", zap.Error(err))
		} else {
			for _, agent := range multicastAgents {
				if !seen[agent.Card.Name] && p.matchesFilter(agent, filter) {
					agents = append(agents, agent)
					seen[agent.Card.Name] = true
				}
			}
		}
	}

	p.logger.Debug("discovery completed", zap.Int("agents", len(agents)))

	return agents, nil
}

// Subscribe subscribes to agent announcements.
func (p *DiscoveryProtocol) Subscribe(handler func(*AgentInfo)) string {
	p.handlerMu.Lock()
	defer p.handlerMu.Unlock()

	p.handlerSeq++
	id := fmt.Sprintf("handler-%d", p.handlerSeq)
	p.handlers[id] = handler
	return id
}

// Unsubscribe unsubscribes from agent announcements.
func (p *DiscoveryProtocol) Unsubscribe(subscriptionID string) {
	p.handlerMu.Lock()
	defer p.handlerMu.Unlock()

	delete(p.handlers, subscriptionID)
}

// startHTTPServer starts the HTTP discovery server.
func (p *DiscoveryProtocol) startHTTPServer() error {
	p.httpMux = http.NewServeMux()

	// Discovery endpoint
	p.httpMux.HandleFunc("/discovery/agents", p.handleListAgents)
	p.httpMux.HandleFunc("/discovery/agents/", p.handleGetAgent)
	p.httpMux.HandleFunc("/discovery/announce", p.handleAnnounce)
	p.httpMux.HandleFunc("/discovery/health", p.handleHealth)

	addr := fmt.Sprintf("%s:%d", p.config.HTTPHost, p.config.HTTPPort)
	p.httpServer = &http.Server{
		Addr:         addr,
		Handler:      p.httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			p.logger.Error("HTTP server error", zap.Error(err))
		}
	}()

	p.logger.Info("HTTP discovery server started", zap.String("addr", addr))
	return nil
}

// handleListAgents handles GET /discovery/agents
func (p *DiscoveryProtocol) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Parse filter from query params
	filter := &DiscoveryFilter{}
	if caps := r.URL.Query().Get("capabilities"); caps != "" {
		filter.Capabilities = splitAndTrim(caps, ",")
	}
	if tags := r.URL.Query().Get("tags"); tags != "" {
		filter.Tags = splitAndTrim(tags, ",")
	}

	agents, err := p.Discover(ctx, filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// handleGetAgent handles GET /discovery/agents/{id}
func (p *DiscoveryProtocol) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path
	agentID := r.URL.Path[len("/discovery/agents/"):]
	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check local agents first
	p.localMu.RLock()
	agent, exists := p.localAgents[agentID]
	p.localMu.RUnlock()

	if !exists && p.registry != nil {
		var err error
		agent, err = p.registry.GetAgent(ctx, agentID)
		if err != nil {
			http.Error(w, "agent not found", http.StatusNotFound)
			return
		}
	}

	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agent)
}

// handleAnnounce handles POST /discovery/announce
func (p *DiscoveryProtocol) handleAnnounce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	var info AgentInfo
	if err := json.Unmarshal(body, &info); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := p.Announce(ctx, &info); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// handleHealth handles GET /discovery/health
func (p *DiscoveryProtocol) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy"}`))
}

// startMulticast starts the multicast listener.
func (p *DiscoveryProtocol) startMulticast() error {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", p.config.MulticastAddress, p.config.MulticastPort))
	if err != nil {
		return fmt.Errorf("failed to resolve multicast address: %w", err)
	}
	p.multicastAddr = addr

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on multicast: %w", err)
	}
	p.multicastConn = conn

	// Set read buffer
	conn.SetReadBuffer(65536)

	// Start listener
	p.wg.Add(1)
	go p.multicastListener()

	p.logger.Info("multicast discovery started",
		zap.String("address", p.config.MulticastAddress),
		zap.Int("port", p.config.MulticastPort),
	)

	return nil
}

// multicastListener listens for multicast announcements.
func (p *DiscoveryProtocol) multicastListener() {
	defer p.wg.Done()

	buf := make([]byte, 65536)
	for {
		select {
		case <-p.done:
			return
		default:
			p.multicastConn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := p.multicastConn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				p.logger.Debug("multicast read error", zap.Error(err))
				continue
			}

			// Parse announcement
			var info AgentInfo
			if err := json.Unmarshal(buf[:n], &info); err != nil {
				p.logger.Debug("failed to parse multicast announcement", zap.Error(err))
				continue
			}

			// Process announcement
			p.processMulticastAnnouncement(&info)
		}
	}
}

// announceMulticast sends an announcement via multicast.
func (p *DiscoveryProtocol) announceMulticast(info *AgentInfo) error {
	if p.multicastConn == nil || p.multicastAddr == nil {
		return fmt.Errorf("multicast not initialized")
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal agent info: %w", err)
	}

	_, err = p.multicastConn.WriteToUDP(data, p.multicastAddr)
	return err
}

// processMulticastAnnouncement processes a received multicast announcement.
func (p *DiscoveryProtocol) processMulticastAnnouncement(info *AgentInfo) {
	if info == nil || info.Card == nil {
		return
	}

	info.IsLocal = false
	agentID := info.Card.Name

	// Store in local cache
	p.localMu.Lock()
	p.localAgents[agentID] = info
	p.localMu.Unlock()

	// Register with registry
	if p.registry != nil {
		ctx := context.Background()
		if err := p.registry.RegisterAgent(ctx, info); err != nil {
			// Try update
			p.registry.UpdateAgent(ctx, info)
		}
	}

	// Notify handlers
	p.notifyHandlers(info)

	p.logger.Debug("received multicast announcement", zap.String("agent_id", agentID))
}

// discoverMulticast discovers agents via multicast.
func (p *DiscoveryProtocol) discoverMulticast(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	// For now, return cached multicast discoveries
	// In a full implementation, this would send a discovery request
	p.localMu.RLock()
	defer p.localMu.RUnlock()

	agents := make([]*AgentInfo, 0)
	for _, agent := range p.localAgents {
		if !agent.IsLocal && p.matchesFilter(agent, filter) {
			agents = append(agents, agent)
		}
	}

	return agents, nil
}

// discoverLocal discovers local agents.
func (p *DiscoveryProtocol) discoverLocal(filter *DiscoveryFilter) []*AgentInfo {
	p.localMu.RLock()
	defer p.localMu.RUnlock()

	agents := make([]*AgentInfo, 0)
	for _, agent := range p.localAgents {
		if agent.IsLocal && p.matchesFilter(agent, filter) {
			agents = append(agents, agent)
		}
	}

	return agents
}

// matchesFilter checks if an agent matches the filter.
func (p *DiscoveryProtocol) matchesFilter(agent *AgentInfo, filter *DiscoveryFilter) bool {
	if filter == nil {
		return true
	}

	// Check local filter
	if filter.Local != nil {
		if *filter.Local && !agent.IsLocal {
			return false
		}
	}

	// Check remote filter
	if filter.Remote != nil {
		if *filter.Remote && agent.IsLocal {
			return false
		}
	}

	// Check status filter
	if len(filter.Status) > 0 {
		matched := false
		for _, status := range filter.Status {
			if agent.Status == status {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check capabilities filter
	if len(filter.Capabilities) > 0 {
		for _, reqCap := range filter.Capabilities {
			found := false
			for _, agentCap := range agent.Capabilities {
				if agentCap.Capability.Name == reqCap {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Check tags filter
	if len(filter.Tags) > 0 {
		for _, reqTag := range filter.Tags {
			found := false
			for _, agentCap := range agent.Capabilities {
				for _, tag := range agentCap.Tags {
					if tag == reqTag {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	return true
}

// notifyHandlers notifies all registered handlers of an agent announcement.
func (p *DiscoveryProtocol) notifyHandlers(info *AgentInfo) {
	p.handlerMu.RLock()
	handlers := make([]func(*AgentInfo), 0, len(p.handlers))
	for _, h := range p.handlers {
		handlers = append(handlers, h)
	}
	p.handlerMu.RUnlock()

	for _, handler := range handlers {
		go handler(info)
	}
}

// splitAndTrim splits a string and trims whitespace from each part.
func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, part := range bytes.Split([]byte(s), []byte(sep)) {
		trimmed := bytes.TrimSpace(part)
		if len(trimmed) > 0 {
			parts = append(parts, string(trimmed))
		}
	}
	return parts
}

// DiscoverRemote discovers agents from a remote discovery server.
func (p *DiscoveryProtocol) DiscoverRemote(ctx context.Context, serverURL string, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	// Build URL with query params
	url := serverURL + "/discovery/agents"
	if filter != nil {
		params := make([]string, 0)
		if len(filter.Capabilities) > 0 {
			params = append(params, "capabilities="+joinStrings(filter.Capabilities, ","))
		}
		if len(filter.Tags) > 0 {
			params = append(params, "tags="+joinStrings(filter.Tags, ","))
		}
		if len(params) > 0 {
			url += "?" + joinStrings(params, "&")
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	client := &http.Client{Timeout: p.config.DiscoveryTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// Parse response
	var agents []*AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return agents, nil
}

// AnnounceRemote announces an agent to a remote discovery server.
func (p *DiscoveryProtocol) AnnounceRemote(ctx context.Context, serverURL string, info *AgentInfo) error {
	url := serverURL + "/discovery/announce"

	// Serialize agent info
	body, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal agent info: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	client := &http.Client{Timeout: p.config.DiscoveryTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// joinStrings joins strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// Ensure DiscoveryProtocol implements Protocol interface.
var _ Protocol = (*DiscoveryProtocol)(nil)

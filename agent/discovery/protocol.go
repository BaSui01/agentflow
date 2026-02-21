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

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"go.uber.org/zap"
)

// 发现协议是协议界面的默认执行.
// 它支持本地(正在处理),HTTP,以及多播发现.
type DiscoveryProtocol struct {
	config   *ProtocolConfig
	registry Registry
	logger   *zap.Logger

	// 本地发现
	localAgents map[string]*AgentInfo
	localMu     sync.RWMutex

	// HTTP 远程发现服务器
	httpServer *http.Server
	httpMux    *http.ServeMux

	// 多播发现
	multicastConn *net.UDPConn
	multicastAddr *net.UDPAddr

	// 事件处理器
	handlers   map[string]func(*AgentInfo)
	handlerMu  sync.RWMutex
	handlerSeq int

	// 状态
	running   bool
	runMu     sync.Mutex // 保护 running 字段的并发读写
	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// 协议Config持有发现协议的配置.
type ProtocolConfig struct {
	// 启用本地启用本地( 正在处理中) 发现 。
	EnableLocal bool `json:"enable_local"`

	// 启用 HTTP 启用基于 HTTP 的远程发现 。
	EnableHTTP bool `json:"enable_http"`

	// HTTPPort是HTTP发现服务器的端口.
	HTTPPort int `json:"http_port"`

	// HTTPHost是HTTP发现服务器的主机.
	HTTPHost string `json:"http_host"`

	// 启用多播可以基于多播的发现.
	EnableMulticast bool `json:"enable_multicast"`

	// 多播Address是多播组地址.
	MulticastAddress string `json:"multicast_address"`

	// 多播口是多播口.
	MulticastPort int `json:"multicast_port"`

	// 公告Interval是定期公告的间隔.
	AnnounceInterval time.Duration `json:"announce_interval"`

	// 发现 超时是发现操作的超时.
	DiscoveryTimeout time.Duration `json:"discovery_timeout"`

	// MaxPeers是跟踪的最大对等者数量.
	MaxPeers int `json:"max_peers"`
}

// 默认协议 Config 返回带有合理默认的协议 Config 。
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

// 新发现协议创建了新的发现协议.
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

// 启动发现协议 。
func (p *DiscoveryProtocol) Start(ctx context.Context) error {
	p.runMu.Lock()
	if p.running {
		p.runMu.Unlock()
		return fmt.Errorf("protocol already running")
	}

	// 启用时启动 HTTP 服务器
	if p.config.EnableHTTP {
		if err := p.startHTTPServer(); err != nil {
			p.runMu.Unlock()
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
	}

	// 启用后启动多播收听器
	if p.config.EnableMulticast {
		if err := p.startMulticast(); err != nil {
			p.logger.Warn("failed to start multicast", zap.Error(err))
			// 如果多播失败, 不要失败
		}
	}

	p.running = true
	p.runMu.Unlock()

	p.logger.Info("discovery protocol started",
		zap.Bool("http", p.config.EnableHTTP),
		zap.Bool("multicast", p.config.EnableMulticast),
	)

	return nil
}

// 停止停止发现协议。
func (p *DiscoveryProtocol) Stop(ctx context.Context) error {
	p.runMu.Lock()
	if !p.running {
		p.runMu.Unlock()
		return nil
	}
	p.runMu.Unlock()

	p.closeOnce.Do(func() { close(p.done) })

	// 停止 HTTP 服务器
	if p.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := p.httpServer.Shutdown(shutdownCtx); err != nil {
			p.logger.Error("failed to shutdown HTTP server", zap.Error(err))
		}
	}

	// 停止多播
	if p.multicastConn != nil {
		p.multicastConn.Close()
	}

	p.wg.Wait()

	p.runMu.Lock()
	p.running = false
	p.runMu.Unlock()

	p.logger.Info("discovery protocol stopped")

	return nil
}

// 公告向网络宣布本地代理.
func (p *DiscoveryProtocol) Announce(ctx context.Context, info *AgentInfo) error {
	if info == nil || info.Card == nil {
		return fmt.Errorf("invalid agent info")
	}

	agentID := info.Card.Name

	// 本地注册
	if p.config.EnableLocal {
		p.localMu.Lock()
		p.localAgents[agentID] = info
		p.localMu.Unlock()
	}

	// 向登记册登记
	if p.registry != nil {
		if err := p.registry.RegisterAgent(ctx, info); err != nil {
			// 如果已经注册, 请尝试更新
			if updateErr := p.registry.UpdateAgent(ctx, info); updateErr != nil {
				return fmt.Errorf("failed to register/update agent: %w", err)
			}
		}
	}

	// 如果启用, 通过多播宣告
	if p.config.EnableMulticast && p.multicastConn != nil {
		if err := p.announceMulticast(info); err != nil {
			p.logger.Warn("failed to announce via multicast", zap.Error(err))
		}
	}

	p.logger.Debug("agent announced", zap.String("agent_id", agentID))

	// 通知处理者
	p.notifyHandlers(info)

	return nil
}

// 发现在网络上发现了特工.
func (p *DiscoveryProtocol) Discover(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	agents := make([]*AgentInfo, 0)
	seen := make(map[string]bool)

	// 发现本地特工
	if p.config.EnableLocal {
		localAgents := p.discoverLocal(filter)
		for _, agent := range localAgents {
			if !seen[agent.Card.Name] {
				agents = append(agents, agent)
				seen[agent.Card.Name] = true
			}
		}
	}

	// 从登记处发现
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

	// 如果启用, 通过多播发现
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

// 订阅代理通知 。
func (p *DiscoveryProtocol) Subscribe(handler func(*AgentInfo)) string {
	p.handlerMu.Lock()
	defer p.handlerMu.Unlock()

	p.handlerSeq++
	id := fmt.Sprintf("handler-%d", p.handlerSeq)
	p.handlers[id] = handler
	return id
}

// 从代理通知中取消订阅。
func (p *DiscoveryProtocol) Unsubscribe(subscriptionID string) {
	p.handlerMu.Lock()
	defer p.handlerMu.Unlock()

	delete(p.handlers, subscriptionID)
}

// 启动HTTPServer启动HTTP发现服务器.
func (p *DiscoveryProtocol) startHTTPServer() error {
	p.httpMux = http.NewServeMux()

	// 发现终点
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

// 处理/发现/代理
func (p *DiscoveryProtocol) handleListAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// 从查询参数解析过滤器
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

// handleGet Agent hands 获取/发现/代理/{id}
func (p *DiscoveryProtocol) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 从路径提取代理 ID
	agentID := r.URL.Path[len("/discovery/agents/"):]
	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// 先检查一下本地特工
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

// 通知手柄 POST/发现/通知
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

// 获得/发现/健康
func (p *DiscoveryProtocol) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy"}`))
}

// 启动多收听器。
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

	// 设定读取缓冲
	conn.SetReadBuffer(65536)

	// 开始收听器
	p.wg.Add(1)
	go p.multicastListener()

	p.logger.Info("multicast discovery started",
		zap.String("address", p.config.MulticastAddress),
		zap.Int("port", p.config.MulticastPort),
	)

	return nil
}

// 多播听众收听多播公告.
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

			// 解析通知
			var info AgentInfo
			if err := json.Unmarshal(buf[:n], &info); err != nil {
				p.logger.Debug("failed to parse multicast announcement", zap.Error(err))
				continue
			}

			// 进程通知
			p.processMulticastAnnouncement(&info)
		}
	}
}

// 宣布多播通过多播发送公告.
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

// 处理多播通知。
func (p *DiscoveryProtocol) processMulticastAnnouncement(info *AgentInfo) {
	if info == nil || info.Card == nil {
		return
	}

	info.IsLocal = false
	agentID := info.Card.Name

	// 在本地缓存中存储
	p.localMu.Lock()
	p.localAgents[agentID] = info
	p.localMu.Unlock()

	// 向登记册登记
	if p.registry != nil {
		ctx := context.Background()
		if err := p.registry.RegisterAgent(ctx, info); err != nil {
			// 尝试更新
			p.registry.UpdateAgent(ctx, info)
		}
	}

	// 通知处理者
	p.notifyHandlers(info)

	p.logger.Debug("received multicast announcement", zap.String("agent_id", agentID))
}

// 发现多播通过多播发现代理.
func (p *DiscoveryProtocol) discoverMulticast(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	// 现在,返回缓存的多播发现
	// 在全面实施过程中,这将发出一个发现请求
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

// 发现本地代理。
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

// 匹配过滤器 。
func (p *DiscoveryProtocol) matchesFilter(agent *AgentInfo, filter *DiscoveryFilter) bool {
	if filter == nil {
		return true
	}

	// 检查本地过滤器
	if filter.Local != nil {
		if *filter.Local && !agent.IsLocal {
			return false
		}
	}

	// 检查远程过滤器
	if filter.Remote != nil {
		if *filter.Remote && agent.IsLocal {
			return false
		}
	}

	// 检查状态过滤器
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

	// 检查能力过滤器
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

	// 检查标签过滤器
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

// 通知所有登记在册的经办人 通知代理人
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

// 和Trim从每个部分分割出一个字符串并修剪白空间。
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

// DiscoverRemote从远程发现服务器中发现了特工.
func (p *DiscoveryProtocol) DiscoverRemote(ctx context.Context, serverURL string, filter *DiscoveryFilter) ([]*AgentInfo, error) {
	// 以查询参数构建 URL
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

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 执行请求
	client := tlsutil.SecureHTTPClient(p.config.DiscoveryTimeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	// 解析响应
	var agents []*AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return agents, nil
}

// 宣告向远程发现服务器发布代理消息.
func (p *DiscoveryProtocol) AnnounceRemote(ctx context.Context, serverURL string, info *AgentInfo) error {
	url := serverURL + "/discovery/announce"

	// 序列化代理信息
	body, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("failed to marshal agent info: %w", err)
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// 执行请求
	client := tlsutil.SecureHTTPClient(p.config.DiscoveryTimeout)
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

// 加入 Strings 用分隔符加入字符串 。
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

// 确保发现协议执行协议接口。
var _ Protocol = (*DiscoveryProtocol)(nil)

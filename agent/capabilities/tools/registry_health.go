package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

type HealthChecker struct {
	config   *HealthCheckerConfig
	registry *CapabilityRegistry
	logger   *zap.Logger

	// httpClient 共享的健康检查 HTTP 客户端
	httpClient *http.Client

	// 失败 。
	failureCounts map[string]int
	failureMu     sync.Mutex

	done      chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// 健康检查员Config拥有健康检查员的配置.
type HealthCheckerConfig struct {
	// 间距是指健康检查之间的间隔.
	Interval time.Duration

	// 暂停是健康检查的暂停。
	Timeout time.Duration

	// 体质不健康 阈值是标记不健康前连续失败的次数.
	UnhealthyThreshold int
}

// 新健康检查器创造了一个新的健康检查器。
func NewHealthChecker(config *HealthCheckerConfig, registry *CapabilityRegistry, logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		config:        config,
		registry:      registry,
		logger:        logger.With(zap.String("component", "health_checker")),
		httpClient:    tlsutil.SecureHTTPClient(5 * time.Second),
		failureCounts: make(map[string]int),
		done:          make(chan struct{}),
	}
}

// 开始体检
func (h *HealthChecker) Start(ctx context.Context) error {
	h.wg.Add(1)
	go h.run()
	h.logger.Info("health checker started")
	return nil
}

// 停止停止健康检查。
func (h *HealthChecker) Stop(ctx context.Context) error {
	h.closeOnce.Do(func() { close(h.done) })
	h.wg.Wait()
	h.logger.Info("health checker stopped")
	return nil
}

// 运行是主要的健康检查循环。
func (h *HealthChecker) run() {
	defer h.wg.Done()

	ticker := time.NewTicker(h.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.checkAll()
		case <-h.done:
			return
		}
	}
}

// 对所有注册代理人进行健康检查。
func (h *HealthChecker) checkAll() {
	ctx, cancel := context.WithTimeout(context.Background(), h.config.Timeout)
	defer cancel()

	agents, err := h.registry.ListAgents(ctx)
	if err != nil {
		h.logger.Error("failed to list agents for health check", zap.Error(err))
		return
	}

	for _, agent := range agents {
		h.checkAgent(ctx, agent)
	}
}

// 代理对单一代理进行健康检查。
func (h *HealthChecker) checkAgent(ctx context.Context, agent *AgentInfo) {
	agentID := agent.Card.Name
	result := h.performHealthCheck(ctx, agent)

	h.failureMu.Lock()
	defer h.failureMu.Unlock()

	if result.Healthy {
		// 重设失败取决于成功
		if h.failureCounts[agentID] > 0 {
			h.logger.Info("agent health recovered",
				zap.String("agent_id", agentID),
			)
			h.registry.emitEvent(&DiscoveryEvent{
				Type:      DiscoveryEventHealthCheckRecovered,
				AgentID:   agentID,
				Timestamp: time.Now(),
			})
		}
		h.failureCounts[agentID] = 0
		if err := h.registry.UpdateAgentStatus(ctx, agentID, AgentStatusOnline); err != nil {
			h.logger.Warn("failed to mark agent online after health recovery",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
		}
	} else {
		h.failureCounts[agentID]++
		h.logger.Warn("agent health check failed",
			zap.String("agent_id", agentID),
			zap.Int("consecutive_failures", h.failureCounts[agentID]),
			zap.String("message", result.Message),
		)

		if h.failureCounts[agentID] >= h.config.UnhealthyThreshold {
			if err := h.registry.UpdateAgentStatus(ctx, agentID, AgentStatusUnhealthy); err != nil {
				h.logger.Warn("failed to mark agent unhealthy after health check failure",
					zap.String("agent_id", agentID),
					zap.Error(err),
				)
			}
			h.registry.emitEvent(&DiscoveryEvent{
				Type:      DiscoveryEventHealthCheckFailed,
				AgentID:   agentID,
				Timestamp: time.Now(),
				Data:      mustMarshal(result),
			})
		}
	}
}

// 进行健康检查
func (h *HealthChecker) performHealthCheck(ctx context.Context, agent *AgentInfo) *HealthCheckResult {
	start := time.Now()
	result := &HealthCheckResult{
		AgentID:   agent.Card.Name,
		Timestamp: start,
	}

	// 对本地特工,请检查他们是否还在登记和反应
	if agent.IsLocal {
		// 检查心跳新鲜度
		if time.Since(agent.LastHeartbeat) > h.config.Interval*3 {
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = "heartbeat timeout"
		} else {
			result.Healthy = true
			result.Status = AgentStatusOnline
		}
		result.Latency = time.Since(start)
		return result
	}

	// 对远程特工进行HTTP健康检查
	if agent.Endpoint != "" {
		healthURL := strings.TrimRight(agent.Endpoint, "/") + "/health"
		client := h.httpClient
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = fmt.Sprintf("failed to create health check request: %v", err)
			result.Latency = time.Since(start)
			return result
		}
		resp, err := client.Do(req)
		if err != nil {
			if resp != nil {
				resp.Body.Close()
			}
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = fmt.Sprintf("health check request failed: %v", err)
			result.Latency = time.Since(start)
			return result
		}
		resp.Body.Close()
		result.Latency = time.Since(start)
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.Healthy = true
			result.Status = AgentStatusOnline
		} else {
			result.Healthy = false
			result.Status = AgentStatusUnhealthy
			result.Message = fmt.Sprintf("health check returned status %d", resp.StatusCode)
		}
		return result
	}

	result.Healthy = true
	result.Status = AgentStatusOnline
	result.Latency = time.Since(start)
	return result
}

// must Marshal 向 JSON 输入数据, 错误时返回零 。
func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// 确保能力登记工具注册界面。
var _ Registry = (*CapabilityRegistry)(nil)

package llm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

type HealthMonitor struct {
	mu          sync.RWMutex
	db          *gorm.DB
	healthScore map[string]float64             // provider_code -> score (0-1)
	qpsCounter  map[string]*QPSCounter         // provider_code -> QPS counter
	probe       map[string]ProviderProbeResult // provider_code -> active probe result
	ctx         context.Context
	cancel      context.CancelFunc
}

type QPSCounter struct {
	lastSec atomic.Int64
	buckets [60]atomic.Int64
	maxQPS  atomic.Int64 // 配置的最大 QPS（0 表示无限制）
}

type ProviderHealthStats struct {
	ProviderCode string
	HealthScore  float64
	CurrentQPS   int
	ErrorRate    float64
	LatencyP95   time.Duration
	LastCheckAt  time.Time
}

type ProviderProbeResult struct {
	Healthy     bool
	Latency     time.Duration
	ErrorRate   float64
	LastError   string
	LastCheckAt time.Time
}

func NewHealthMonitor(db *gorm.DB) *HealthMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	monitor := &HealthMonitor{
		db:          db,
		healthScore: make(map[string]float64),
		qpsCounter:  make(map[string]*QPSCounter),
		probe:       make(map[string]ProviderProbeResult),
		ctx:         ctx,
		cancel:      cancel,
	}

	// 启动后台健康检查循环
	go monitor.startHealthCheckLoop()

	return monitor
}

func (m *HealthMonitor) Stop() {
	m.cancel()
}

// GetHealthScore 获取 Provider 的健康分数 (0-1)
// 使用写锁，因为 getCurrentQPSUnsafe 内部调用 bumpWindow 会修改计数器状态。
func (m *HealthMonitor) GetHealthScore(providerCode string) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if probe, ok := m.probe[providerCode]; ok && !probe.Healthy {
		return 0.0 // 主动探活失败，直接熔断
	}

	if counter, exists := m.qpsCounter[providerCode]; exists && counter.maxQPS.Load() > 0 {
		currentQPS := m.getCurrentQPSUnsafe(providerCode)
		if currentQPS >= int(counter.maxQPS.Load()) {
			return 0.0 // QPS 超限，标记为不健康
		}
	}

	if score, exists := m.healthScore[providerCode]; exists {
		return score
	}
	return 1.0 // 默认健康
}

// GetCurrentQPS 获取当前 QPS
// 使用写锁，因为 getCurrentQPSUnsafe 内部调用 bumpWindow 会修改计数器状态。
func (m *HealthMonitor) GetCurrentQPS(providerCode string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getCurrentQPSUnsafe(providerCode)
}

func (m *HealthMonitor) getCurrentQPSUnsafe(providerCode string) int {
	counter, exists := m.qpsCounter[providerCode]
	if !exists {
		return 0
	}
	now := time.Now()
	counter.bumpWindow(now.Unix())
	var total int64
	for i := range counter.buckets {
		total += counter.buckets[i].Load()
	}
	if total < 0 {
		return 0
	}
	return int(total)
}

// IncrementQPS 记录一次请求
func (m *HealthMonitor) IncrementQPS(providerCode string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.qpsCounter[providerCode]; !exists {
		m.qpsCounter[providerCode] = newQPSCounter(time.Now())
	}

	counter := m.qpsCounter[providerCode]
	now := time.Now().Unix()
	counter.bumpWindow(now)
	counter.buckets[now%60].Add(1)
}

// SetMaxQPS 设置 Provider 的最大 QPS（0 表示无限制）
func (m *HealthMonitor) SetMaxQPS(providerCode string, maxQPS int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.qpsCounter[providerCode]; !exists {
		m.qpsCounter[providerCode] = newQPSCounter(time.Now())
	}
	m.qpsCounter[providerCode].maxQPS.Store(int64(maxQPS))
}

// GetAllProviderStats 获取所有 Provider 的健康统计
// 使用写锁，因为 getCurrentQPSUnsafe 内部调用 bumpWindow 会修改计数器状态。
func (m *HealthMonitor) GetAllProviderStats() []ProviderHealthStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := make([]ProviderHealthStats, 0, len(m.healthScore))
	for providerCode, score := range m.healthScore {
		lastCheckAt := time.Now()
		if probe, ok := m.probe[providerCode]; ok && !probe.LastCheckAt.IsZero() {
			lastCheckAt = probe.LastCheckAt
		}
		stats = append(stats, ProviderHealthStats{
			ProviderCode: providerCode,
			HealthScore:  score,
			CurrentQPS:   m.getCurrentQPSUnsafe(providerCode),
			LastCheckAt:  lastCheckAt,
		})
	}
	return stats
}

func (m *HealthMonitor) UpdateProbe(providerCode string, st *HealthStatus, err error) {
	if providerCode == "" {
		return
	}
	now := time.Now()
	res := ProviderProbeResult{Healthy: false, LastCheckAt: now}
	if st != nil {
		res.Healthy = st.Healthy
		res.Latency = st.Latency
		res.ErrorRate = st.ErrorRate
	}
	if err != nil {
		res.Healthy = false
		res.LastError = err.Error()
	}
	m.mu.Lock()
	m.probe[providerCode] = res
	m.mu.Unlock()
}

// startHealthCheckLoop 后台健康检查循环（每 60 秒）
func (m *HealthMonitor) startHealthCheckLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateAllProviderHealth()
		}
	}
}

func newQPSCounter(now time.Time) *QPSCounter {
	c := &QPSCounter{}
	c.lastSec.Store(now.Unix())
	c.maxQPS.Store(0)
	return c
}

func (c *QPSCounter) bumpWindow(nowSec int64) {
	prev := c.lastSec.Load()
	for nowSec > prev {
		if c.lastSec.CompareAndSwap(prev, nowSec) {
			gap := nowSec - prev
			if gap >= 60 {
				for i := range c.buckets {
					c.buckets[i].Store(0)
				}
				return
			}
			for s := prev + 1; s <= nowSec; s++ {
				c.buckets[s%60].Store(0)
			}
			return
		}
		prev = c.lastSec.Load()
	}
}

// updateAllProviderHealth 更新所有 Provider 的健康分数
func (m *HealthMonitor) updateAllProviderHealth() {
	var providers []struct {
		Code string
	}

	m.db.Table("sc_llm_providers").
		Select("code").
		Where("status = ?", 1).
		Find(&providers)

	for _, p := range providers {
		score := m.calculateHealthScore(p.Code)
		m.mu.Lock()
		m.healthScore[p.Code] = score
		m.mu.Unlock()
	}
}

// calculateHealthScore 计算单个 Provider 的健康分数
func (m *HealthMonitor) calculateHealthScore(providerCode string) float64 {
	// 查询最近 5 分钟的统计数据
	since := time.Now().Add(-5 * time.Minute)

	var stats struct {
		TotalCalls  int
		FailedCalls int
		AvgLatency  float64
	}

	m.db.Table("sc_llm_usage_logs").
		Select("COUNT(*) as total_calls, SUM(CASE WHEN status = 0 THEN 1 ELSE 0 END) as failed_calls, AVG(latency_ms) as avg_latency").
		Where("provider = ? AND created_at >= ?", providerCode, since).
		Scan(&stats)

	if stats.TotalCalls == 0 {
		return 1.0 // 无数据，默认健康
	}

	errorRate := float64(stats.FailedCalls) / float64(stats.TotalCalls)

	// 健康分数计算：
	// - 错误率 < 1%: 1.0
	// - 错误率 1-5%: 0.8
	// - 错误率 5-10%: 0.5
	// - 错误率 > 10%: 0.2
	score := 1.0
	if errorRate > 0.01 {
		score = 0.8
	}
	if errorRate > 0.05 {
		score = 0.5
	}
	if errorRate > 0.10 {
		score = 0.2
	}

	// 延迟因子（P95 估算）
	latencyP95 := stats.AvgLatency * 1.2
	if latencyP95 > 5000 { // 超过 5 秒
		score *= 0.5
	} else if latencyP95 > 3000 { // 超过 3 秒
		score *= 0.8
	}

	return score
}

// ForceHealthCheck 强制立即检查指定 Provider 的健康状态
func (m *HealthMonitor) ForceHealthCheck(providerCode string) error {
	var provider struct {
		Code string
	}

	err := m.db.Table("sc_llm_providers").
		Select("code").
		Where("code = ?", providerCode).
		First(&provider).Error

	if err != nil {
		return fmt.Errorf("provider not found: %w", err)
	}

	score := m.calculateHealthScore(provider.Code)
	m.mu.Lock()
	m.healthScore[provider.Code] = score
	m.mu.Unlock()

	return nil
}

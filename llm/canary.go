package llm

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"gorm.io/gorm"
)

type CanaryStage string

const (
	CanaryStageInit     CanaryStage = "init"
	CanaryStage10Pct    CanaryStage = "10pct"
	CanaryStage50Pct    CanaryStage = "50pct"
	CanaryStage100Pct   CanaryStage = "100pct"
	CanaryStageRollback CanaryStage = "rollback"
)

type CanaryConfig struct {
	mu          sync.RWMutex
	db          *gorm.DB
	deployments map[uint]*CanaryDeployment // provider_id -> deployment
	ctx         context.Context
	cancel      context.CancelFunc
}

type CanaryDeployment struct {
	ID             uint
	ProviderID     uint
	CanaryVersion  string
	StableVersion  string
	TrafficPercent int
	Stage          CanaryStage
	StartTime      time.Time
	MaxErrorRate   float64
	MaxLatencyP95  time.Duration
	AutoRollback   bool
	RollbackReason string
}

type ProviderStats struct {
	ErrorRate   float64
	LatencyP95  time.Duration
	TotalCalls  int
	FailedCalls int
}

func NewCanaryConfig(db *gorm.DB) *CanaryConfig {
	ctx, cancel := context.WithCancel(context.Background())
	config := &CanaryConfig{
		db:          db,
		deployments: make(map[uint]*CanaryDeployment),
		ctx:         ctx,
		cancel:      cancel,
	}

	config.loadFromDB()
	return config
}

func (c *CanaryConfig) Stop() {
	c.cancel()
}

// loadFromDB 从数据库加载活跃的金丝雀部署
func (c *CanaryConfig) loadFromDB() {
	// 如果 db 为零, 请跳过( 如在测试中)
	if c.db == nil {
		return
	}

	var records []struct {
		ID             uint
		ProviderID     uint
		CanaryVersion  string
		StableVersion  string
		TrafficPercent int
		Stage          string
		StartedAt      time.Time
		MaxErrorRate   float64
		MaxLatencyP95  int
		AutoRollback   bool
		RollbackReason string
	}

	c.db.Table("sc_llm_canary_deployments").
		Where("stage NOT IN ('100pct', 'rollback') OR (stage = 'rollback' AND updated_at >= ?)", time.Now().Add(-24*time.Hour)).
		Find(&records)

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range records {
		c.deployments[r.ProviderID] = &CanaryDeployment{
			ID:             r.ID,
			ProviderID:     r.ProviderID,
			CanaryVersion:  r.CanaryVersion,
			StableVersion:  r.StableVersion,
			TrafficPercent: r.TrafficPercent,
			Stage:          CanaryStage(r.Stage),
			StartTime:      r.StartedAt,
			MaxErrorRate:   r.MaxErrorRate,
			MaxLatencyP95:  time.Duration(r.MaxLatencyP95) * time.Millisecond,
			AutoRollback:   r.AutoRollback,
			RollbackReason: r.RollbackReason,
		}
	}

	log.Printf("[CanaryConfig] Loaded %d active deployments from database", len(c.deployments))
}

// GetDeployment 获取指定 Provider 的金丝雀部署配置
func (c *CanaryConfig) GetDeployment(providerID uint) *CanaryDeployment {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deployments[providerID]
}

// SetDeployment 设置金丝雀部署
func (c *CanaryConfig) SetDeployment(deployment *CanaryDeployment) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 写入数据库
	record := map[string]interface{}{
		"provider_id":        deployment.ProviderID,
		"canary_version":     deployment.CanaryVersion,
		"stable_version":     deployment.StableVersion,
		"traffic_percent":    deployment.TrafficPercent,
		"stage":              string(deployment.Stage),
		"max_error_rate":     deployment.MaxErrorRate,
		"max_latency_p95_ms": int(deployment.MaxLatencyP95.Milliseconds()),
		"auto_rollback":      deployment.AutoRollback,
		"started_at":         deployment.StartTime,
	}

	if deployment.ID > 0 {
		// 更新现有记录
		c.db.Table("sc_llm_canary_deployments").Where("id = ?", deployment.ID).Updates(record)
	} else {
		// 创建新记录
		var newRecord struct {
			ID uint
		}
		c.db.Table("sc_llm_canary_deployments").Create(record).Scan(&newRecord)
		deployment.ID = newRecord.ID
	}

	c.deployments[deployment.ProviderID] = deployment
	return nil
}

// UpdateStage 更新部署阶段
func (c *CanaryConfig) UpdateStage(providerID uint, newStage CanaryStage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	deployment, exists := c.deployments[providerID]
	if !exists {
		return fmt.Errorf("no canary deployment found for provider %d", providerID)
	}

	oldStage := deployment.Stage
	deployment.Stage = newStage

	// 更新流量百分比
	switch newStage {
	case CanaryStage10Pct:
		deployment.TrafficPercent = 10
	case CanaryStage50Pct:
		deployment.TrafficPercent = 50
	case CanaryStage100Pct:
		deployment.TrafficPercent = 100
	case CanaryStageRollback:
		deployment.TrafficPercent = 0
	}

	// 写入数据库
	updates := map[string]interface{}{
		"stage":           string(newStage),
		"traffic_percent": deployment.TrafficPercent,
	}

	if newStage == CanaryStage100Pct {
		updates["completed_at"] = time.Now()
	}

	c.db.Table("sc_llm_canary_deployments").Where("id = ?", deployment.ID).Updates(updates)

	log.Printf("[CanaryConfig] Provider %d stage updated: %s -> %s (traffic: %d%%)",
		providerID, oldStage, newStage, deployment.TrafficPercent)

	return nil
}

// TriggerRollback 触发回滚
func (c *CanaryConfig) TriggerRollback(providerID uint, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	deployment, exists := c.deployments[providerID]
	if !exists {
		return fmt.Errorf("no canary deployment found for provider %d", providerID)
	}

	deployment.Stage = CanaryStageRollback
	deployment.TrafficPercent = 0
	deployment.RollbackReason = reason

	// 写入数据库
	c.db.Table("sc_llm_canary_deployments").Where("id = ?", deployment.ID).Updates(map[string]interface{}{
		"stage":           "rollback",
		"traffic_percent": 0,
		"rollback_reason": reason,
	})

	// 写入审计日志
	auditLog := AuditLog{
		Action:       "canary_rollback",
		ResourceType: "llm_provider",
		ResourceID:   strconv.FormatUint(uint64(providerID), 10),
		Details: map[string]interface{}{
			"reason":         reason,
			"canary_version": deployment.CanaryVersion,
			"stable_version": deployment.StableVersion,
			"previous_stage": deployment.Stage,
		},
		CreatedAt: time.Now(),
	}
	c.db.Table("sc_audit_logs").Create(&auditLog)

	log.Printf("[ALERT] Canary rollback triggered for provider %d: %s", providerID, reason)

	return nil
}

// RemoveDeployment 移除金丝雀部署（完成或回滚后清理）
func (c *CanaryConfig) RemoveDeployment(providerID uint) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.deployments, providerID)
}

// GetAllDeployments 获取所有活跃的金丝雀部署
func (c *CanaryConfig) GetAllDeployments() []*CanaryDeployment {
	c.mu.RLock()
	defer c.mu.RUnlock()

	deployments := make([]*CanaryDeployment, 0, len(c.deployments))
	for _, d := range c.deployments {
		deployments = append(deployments, d)
	}
	return deployments
}

// =========================================
// 加那利监视器
// =========================================

type CanaryMonitor struct {
	db            *gorm.DB
	canaryConfig  *CanaryConfig
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewCanaryMonitor(db *gorm.DB, canaryConfig *CanaryConfig) *CanaryMonitor {
	ctx, cancel := context.WithCancel(context.Background())
	return &CanaryMonitor{
		db:            db,
		canaryConfig:  canaryConfig,
		checkInterval: 30 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (m *CanaryMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	log.Printf("[CanaryMonitor] Started with check interval: %v", m.checkInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[CanaryMonitor] Stopped")
			return
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkAndRollback()
		}
	}
}

func (m *CanaryMonitor) Stop() {
	m.cancel()
}

func (m *CanaryMonitor) checkAndRollback() {
	deployments := m.canaryConfig.GetAllDeployments()

	for _, deployment := range deployments {
		// 跳过已完成或回滚中的部署
		if deployment.Stage == CanaryStage100Pct || deployment.Stage == CanaryStageRollback {
			continue
		}

		// 查询金丝雀版本的统计数据（最近 5 分钟）
		stats := m.getProviderStats(deployment.ProviderID, deployment.CanaryVersion, 5*time.Minute)

		// 检查是否超过阈值
		shouldRollback := false
		var reason string

		if stats.TotalCalls > 10 && stats.ErrorRate > deployment.MaxErrorRate {
			shouldRollback = true
			reason = fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%% (calls: %d, failures: %d)",
				stats.ErrorRate*100, deployment.MaxErrorRate*100, stats.TotalCalls, stats.FailedCalls)
		}

		if stats.TotalCalls > 10 && stats.LatencyP95 > deployment.MaxLatencyP95 {
			shouldRollback = true
			reason = fmt.Sprintf("P95 latency %v exceeds threshold %v (calls: %d)",
				stats.LatencyP95, deployment.MaxLatencyP95, stats.TotalCalls)
		}

		// 执行自动回滚
		if shouldRollback && deployment.AutoRollback {
			log.Printf("[CanaryMonitor] Auto-rollback triggered for provider %d: %s", deployment.ProviderID, reason)
			m.canaryConfig.TriggerRollback(deployment.ProviderID, reason)
		}
	}
}

func (m *CanaryMonitor) getProviderStats(providerID uint, providerCode string, duration time.Duration) ProviderStats {
	since := time.Now().Add(-duration)

	var result struct {
		TotalCalls  int
		FailedCalls int
		AvgLatency  float64
	}

	m.db.Table("sc_llm_usage_logs").
		Select("COUNT(*) as total_calls, SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as failed_calls, AVG(latency_ms) as avg_latency").
		Where("provider_id = ? AND created_at >= ?", providerID, since).
		Scan(&result)

	errorRate := 0.0
	if result.TotalCalls > 0 {
		errorRate = float64(result.FailedCalls) / float64(result.TotalCalls)
	}

	// 简化：用 avg * 1.2 估算 P95
	latencyP95 := time.Duration(result.AvgLatency*1.2) * time.Millisecond

	return ProviderStats{
		ErrorRate:   errorRate,
		LatencyP95:  latencyP95,
		TotalCalls:  result.TotalCalls,
		FailedCalls: result.FailedCalls,
	}
}

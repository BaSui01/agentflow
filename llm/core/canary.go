package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
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
	logger      *zap.Logger
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

type canaryDeploymentRow struct {
	ID             uint       `gorm:"column:id"`
	ProviderID     uint       `gorm:"column:provider_id"`
	CanaryVersion  string     `gorm:"column:canary_version"`
	StableVersion  string     `gorm:"column:stable_version"`
	TrafficPercent int        `gorm:"column:traffic_percent"`
	Stage          string     `gorm:"column:stage"`
	MaxErrorRate   float64    `gorm:"column:max_error_rate"`
	MaxLatencyP95  int        `gorm:"column:max_latency_p95_ms"`
	AutoRollback   bool       `gorm:"column:auto_rollback"`
	StartedAt      time.Time  `gorm:"column:started_at"`
	CompletedAt    *time.Time `gorm:"column:completed_at"`
	RollbackReason string     `gorm:"column:rollback_reason"`
}

type ProviderStats struct {
	ErrorRate   float64
	LatencyP95  time.Duration
	TotalCalls  int
	FailedCalls int
}

func NewCanaryConfig(db *gorm.DB, logger *zap.Logger) *CanaryConfig {
	if logger == nil {
		logger = zap.NewNop()
	}
	ctx, cancel := context.WithCancel(context.Background())
	config := &CanaryConfig{
		db:          db,
		logger:      logger,
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

	c.logger.Info("loaded active deployments from database", zap.Int("count", len(c.deployments)))
}

// GetDeployment 获取指定 Provider 的金丝雀部署配置
func (c *CanaryConfig) GetDeployment(providerID uint) *CanaryDeployment {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deployments[providerID]
}

// SetDeployment 设置金丝雀部署
func (c *CanaryConfig) SetDeployment(deployment *CanaryDeployment) error {
	if deployment == nil {
		return fmt.Errorf("deployment cannot be nil")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	record := map[string]any{
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

	var createdID uint
	if c.db != nil {
		if err := c.db.Transaction(func(tx *gorm.DB) error {
			if deployment.ID > 0 {
				return tx.Table("sc_llm_canary_deployments").Where("id = ?", deployment.ID).Updates(record).Error
			}

			row := canaryDeploymentRow{
				ProviderID:     deployment.ProviderID,
				CanaryVersion:  deployment.CanaryVersion,
				StableVersion:  deployment.StableVersion,
				TrafficPercent: deployment.TrafficPercent,
				Stage:          string(deployment.Stage),
				MaxErrorRate:   deployment.MaxErrorRate,
				MaxLatencyP95:  int(deployment.MaxLatencyP95.Milliseconds()),
				AutoRollback:   deployment.AutoRollback,
				StartedAt:      deployment.StartTime,
			}
			if err := tx.Table("sc_llm_canary_deployments").Create(&row).Error; err != nil {
				return err
			}
			createdID = row.ID
			return nil
		}); err != nil {
			return err
		}
	}

	if createdID > 0 {
		deployment.ID = createdID
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
	newTrafficPercent := deployment.TrafficPercent
	switch newStage {
	case CanaryStage10Pct:
		newTrafficPercent = 10
	case CanaryStage50Pct:
		newTrafficPercent = 50
	case CanaryStage100Pct:
		newTrafficPercent = 100
	case CanaryStageRollback:
		newTrafficPercent = 0
	}

	updates := map[string]any{
		"stage":           string(newStage),
		"traffic_percent": newTrafficPercent,
	}

	if newStage == CanaryStage100Pct {
		updates["completed_at"] = time.Now()
	}

	if c.db != nil {
		if err := c.db.Transaction(func(tx *gorm.DB) error {
			return tx.Table("sc_llm_canary_deployments").Where("id = ?", deployment.ID).Updates(updates).Error
		}); err != nil {
			return err
		}
	}

	deployment.Stage = newStage
	deployment.TrafficPercent = newTrafficPercent
	c.logger.Info("provider stage updated",
		zap.Uint("providerID", providerID),
		zap.String("from", string(oldStage)),
		zap.String("to", string(newStage)),
		zap.Int("trafficPercent", newTrafficPercent))

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

	previousStage := deployment.Stage
	createdAt := time.Now()
	if c.db != nil {
		detailsJSON, err := json.Marshal(map[string]any{
			"reason":         reason,
			"canary_version": deployment.CanaryVersion,
			"stable_version": deployment.StableVersion,
			"previous_stage": previousStage,
		})
		if err != nil {
			return fmt.Errorf("marshal canary rollback audit details: %w", err)
		}
		if err := c.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Table("sc_llm_canary_deployments").Where("id = ?", deployment.ID).Updates(map[string]any{
				"stage":           "rollback",
				"traffic_percent": 0,
				"rollback_reason": reason,
			}).Error; err != nil {
				return err
			}
			return tx.Table("sc_audit_logs").Create(map[string]any{
				"action":        "canary_rollback",
				"resource_type": "llm_provider",
				"resource_id":   strconv.FormatUint(uint64(providerID), 10),
				"details":       string(detailsJSON),
				"created_at":    createdAt,
			}).Error
		}); err != nil {
			return err
		}
	}

	deployment.Stage = CanaryStageRollback
	deployment.TrafficPercent = 0
	deployment.RollbackReason = reason

	c.logger.Warn("canary rollback triggered",
		zap.Uint("providerID", providerID),
		zap.String("reason", reason))

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
	logger        *zap.Logger
	checkInterval time.Duration
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewCanaryMonitor(db *gorm.DB, canaryConfig *CanaryConfig, logger *zap.Logger) *CanaryMonitor {
	if logger == nil {
		logger = zap.NewNop()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &CanaryMonitor{
		db:            db,
		canaryConfig:  canaryConfig,
		logger:        logger,
		checkInterval: 30 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

func (m *CanaryMonitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()

	m.logger.Info("canary monitor started", zap.Duration("checkInterval", m.checkInterval))

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("canary monitor stopped")
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
			m.logger.Warn("auto-rollback triggered",
				zap.Uint("providerID", deployment.ProviderID),
				zap.String("reason", reason))
			if err := m.canaryConfig.TriggerRollback(deployment.ProviderID, reason); err != nil {
				m.logger.Error("failed to trigger rollback",
					zap.Uint("providerID", deployment.ProviderID),
					zap.Error(err),
				)
			}
		}
	}
}

func (m *CanaryMonitor) getProviderStats(providerID uint, providerCode string, duration time.Duration) ProviderStats {
	if m.db == nil {
		return ProviderStats{}
	}

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

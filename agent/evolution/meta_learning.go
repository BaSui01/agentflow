// Package evolution provides self-evolution and meta-learning capabilities.
package evolution

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Strategy represents an agent strategy configuration.
type Strategy struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  map[string]any    `json:"parameters"`
	Prompts     map[string]string `json:"prompts,omitempty"`
	Version     int               `json:"version"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// StrategyPerformance tracks strategy performance metrics.
type StrategyPerformance struct {
	StrategyID     string    `json:"strategy_id"`
	SuccessRate    float64   `json:"success_rate"`
	AverageLatency float64   `json:"average_latency_ms"`
	AverageTokens  float64   `json:"average_tokens"`
	AverageCost    float64   `json:"average_cost"`
	SampleCount    int       `json:"sample_count"`
	LastUpdated    time.Time `json:"last_updated"`
}

// ExecutionFeedback represents feedback from a single execution.
type ExecutionFeedback struct {
	StrategyID string         `json:"strategy_id"`
	TaskType   string         `json:"task_type"`
	Success    bool           `json:"success"`
	Score      float64        `json:"score"`
	Latency    time.Duration  `json:"latency"`
	Tokens     int            `json:"tokens"`
	Cost       float64        `json:"cost"`
	Error      string         `json:"error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// EvolutionConfig configures the meta-learning system.
type EvolutionConfig struct {
	MinSamplesForEvolution int           `json:"min_samples"`
	EvolutionInterval      time.Duration `json:"evolution_interval"`
	ExplorationRate        float64       `json:"exploration_rate"` // 0.0-1.0
	MaxStrategies          int           `json:"max_strategies"`
	PerformanceWindow      time.Duration `json:"performance_window"`
	AutoEvolve             bool          `json:"auto_evolve"`
}

// DefaultEvolutionConfig returns sensible defaults.
func DefaultEvolutionConfig() EvolutionConfig {
	return EvolutionConfig{
		MinSamplesForEvolution: 50,
		EvolutionInterval:      time.Hour,
		ExplorationRate:        0.1,
		MaxStrategies:          10,
		PerformanceWindow:      24 * time.Hour,
		AutoEvolve:             true,
	}
}

// MetaLearner implements self-evolution through strategy optimization.
type MetaLearner struct {
	config      EvolutionConfig
	strategies  map[string]*Strategy
	performance map[string]*StrategyPerformance
	feedback    []ExecutionFeedback
	logger      *zap.Logger
	mu          sync.RWMutex

	// Active strategy selection
	activeStrategy string
	taskStrategies map[string]string // taskType -> strategyID
}

// NewMetaLearner creates a new meta-learner.
func NewMetaLearner(config EvolutionConfig, logger *zap.Logger) *MetaLearner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &MetaLearner{
		config:         config,
		strategies:     make(map[string]*Strategy),
		performance:    make(map[string]*StrategyPerformance),
		feedback:       make([]ExecutionFeedback, 0),
		taskStrategies: make(map[string]string),
		logger:         logger,
	}
}

// RegisterStrategy registers a new strategy.
func (m *MetaLearner) RegisterStrategy(strategy *Strategy) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.strategies) >= m.config.MaxStrategies {
		// Remove worst performing strategy
		m.removeWorstStrategy()
	}

	strategy.CreatedAt = time.Now()
	strategy.UpdatedAt = time.Now()
	m.strategies[strategy.ID] = strategy

	m.performance[strategy.ID] = &StrategyPerformance{
		StrategyID:  strategy.ID,
		LastUpdated: time.Now(),
	}

	if m.activeStrategy == "" {
		m.activeStrategy = strategy.ID
	}

	m.logger.Info("strategy registered", zap.String("id", strategy.ID))
	return nil
}

// RecordFeedback records execution feedback for learning.
func (m *MetaLearner) RecordFeedback(feedback ExecutionFeedback) {
	m.mu.Lock()
	defer m.mu.Unlock()

	feedback.Timestamp = time.Now()
	m.feedback = append(m.feedback, feedback)

	// Update performance metrics
	m.updatePerformance(feedback)

	// Trigger evolution check if auto-evolve is enabled
	if m.config.AutoEvolve && len(m.feedback)%m.config.MinSamplesForEvolution == 0 {
		go m.evolve()
	}
}

// SelectStrategy selects the best strategy for a task type.
func (m *MetaLearner) SelectStrategy(taskType string) *Strategy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Exploration: randomly try different strategies
	if m.shouldExplore() {
		return m.randomStrategy()
	}

	// Exploitation: use best known strategy
	if strategyID, ok := m.taskStrategies[taskType]; ok {
		if strategy, ok := m.strategies[strategyID]; ok {
			return strategy
		}
	}

	// Fall back to best overall strategy
	return m.bestStrategy()
}

// GetActiveStrategy returns the currently active strategy.
func (m *MetaLearner) GetActiveStrategy() *Strategy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.strategies[m.activeStrategy]
}

// Evolve triggers strategy evolution.
func (m *MetaLearner) Evolve() error {
	return m.evolve()
}

func (m *MetaLearner) evolve() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("starting evolution cycle")

	// Analyze recent feedback
	recentFeedback := m.getRecentFeedback()
	if len(recentFeedback) < m.config.MinSamplesForEvolution {
		m.logger.Debug("not enough samples for evolution",
			zap.Int("samples", len(recentFeedback)),
			zap.Int("required", m.config.MinSamplesForEvolution))
		return nil
	}

	// Update task-strategy mappings based on performance
	m.updateTaskStrategies(recentFeedback)

	// Generate new strategy variants from best performers
	m.generateVariants()

	// Update active strategy
	best := m.bestStrategyLocked()
	if best != nil && best.ID != m.activeStrategy {
		m.logger.Info("switching active strategy",
			zap.String("from", m.activeStrategy),
			zap.String("to", best.ID))
		m.activeStrategy = best.ID
	}

	return nil
}

func (m *MetaLearner) updatePerformance(feedback ExecutionFeedback) {
	perf, ok := m.performance[feedback.StrategyID]
	if !ok {
		return
	}

	// Incremental update of metrics
	n := float64(perf.SampleCount)
	perf.SampleCount++

	if feedback.Success {
		perf.SuccessRate = (perf.SuccessRate*n + 1) / float64(perf.SampleCount)
	} else {
		perf.SuccessRate = (perf.SuccessRate * n) / float64(perf.SampleCount)
	}

	perf.AverageLatency = (perf.AverageLatency*n + float64(feedback.Latency.Milliseconds())) / float64(perf.SampleCount)
	perf.AverageTokens = (perf.AverageTokens*n + float64(feedback.Tokens)) / float64(perf.SampleCount)
	perf.AverageCost = (perf.AverageCost*n + feedback.Cost) / float64(perf.SampleCount)
	perf.LastUpdated = time.Now()
}

func (m *MetaLearner) getRecentFeedback() []ExecutionFeedback {
	cutoff := time.Now().Add(-m.config.PerformanceWindow)
	var recent []ExecutionFeedback
	for _, f := range m.feedback {
		if f.Timestamp.After(cutoff) {
			recent = append(recent, f)
		}
	}
	return recent
}

func (m *MetaLearner) updateTaskStrategies(feedback []ExecutionFeedback) {
	// Group feedback by task type
	taskFeedback := make(map[string][]ExecutionFeedback)
	for _, f := range feedback {
		taskFeedback[f.TaskType] = append(taskFeedback[f.TaskType], f)
	}

	// Find best strategy for each task type
	for taskType, feedbacks := range taskFeedback {
		strategyScores := make(map[string]float64)
		strategyCounts := make(map[string]int)

		for _, f := range feedbacks {
			strategyScores[f.StrategyID] += f.Score
			strategyCounts[f.StrategyID]++
		}

		var bestStrategy string
		var bestScore float64
		for strategyID, totalScore := range strategyScores {
			avgScore := totalScore / float64(strategyCounts[strategyID])
			if avgScore > bestScore {
				bestScore = avgScore
				bestStrategy = strategyID
			}
		}

		if bestStrategy != "" {
			m.taskStrategies[taskType] = bestStrategy
		}
	}
}

func (m *MetaLearner) generateVariants() {
	// Get top performing strategies
	var perfs []*StrategyPerformance
	for _, p := range m.performance {
		perfs = append(perfs, p)
	}

	sort.Slice(perfs, func(i, j int) bool {
		return perfs[i].SuccessRate > perfs[j].SuccessRate
	})

	if len(perfs) == 0 {
		return
	}

	// Generate variant from best strategy
	bestPerf := perfs[0]
	bestStrategy := m.strategies[bestPerf.StrategyID]
	if bestStrategy == nil {
		return
	}

	// Create mutated variant
	variant := &Strategy{
		ID:          fmt.Sprintf("%s_v%d", bestStrategy.ID, bestStrategy.Version+1),
		Name:        bestStrategy.Name + " (evolved)",
		Description: fmt.Sprintf("Evolved from %s", bestStrategy.ID),
		Parameters:  m.mutateParameters(bestStrategy.Parameters),
		Prompts:     bestStrategy.Prompts,
		Version:     bestStrategy.Version + 1,
	}

	// Only add if we have room
	if len(m.strategies) < m.config.MaxStrategies {
		variant.CreatedAt = time.Now()
		variant.UpdatedAt = time.Now()
		m.strategies[variant.ID] = variant
		m.performance[variant.ID] = &StrategyPerformance{
			StrategyID:  variant.ID,
			LastUpdated: time.Now(),
		}
		m.logger.Info("generated strategy variant", zap.String("id", variant.ID))
	}
}

func (m *MetaLearner) mutateParameters(params map[string]any) map[string]any {
	mutated := make(map[string]any)
	for k, v := range params {
		mutated[k] = v
		// Apply small mutations to numeric parameters
		if f, ok := v.(float64); ok {
			mutated[k] = f * (0.9 + 0.2*float64(time.Now().UnixNano()%100)/100)
		}
	}
	return mutated
}

func (m *MetaLearner) shouldExplore() bool {
	// Simple epsilon-greedy exploration
	return float64(time.Now().UnixNano()%1000)/1000 < m.config.ExplorationRate
}

func (m *MetaLearner) randomStrategy() *Strategy {
	for _, s := range m.strategies {
		return s
	}
	return nil
}

func (m *MetaLearner) bestStrategy() *Strategy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.bestStrategyLocked()
}

func (m *MetaLearner) bestStrategyLocked() *Strategy {
	var best *Strategy
	var bestScore float64

	for id, perf := range m.performance {
		if perf.SuccessRate > bestScore {
			bestScore = perf.SuccessRate
			best = m.strategies[id]
		}
	}

	return best
}

func (m *MetaLearner) removeWorstStrategy() {
	var worstID string
	var worstScore float64 = 1.0

	for id, perf := range m.performance {
		if id == m.activeStrategy {
			continue // Don't remove active strategy
		}
		if perf.SuccessRate < worstScore {
			worstScore = perf.SuccessRate
			worstID = id
		}
	}

	if worstID != "" {
		delete(m.strategies, worstID)
		delete(m.performance, worstID)
		m.logger.Info("removed worst strategy", zap.String("id", worstID))
	}
}

// GetPerformanceReport returns a performance report.
func (m *MetaLearner) GetPerformanceReport() map[string]*StrategyPerformance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	report := make(map[string]*StrategyPerformance)
	for k, v := range m.performance {
		report[k] = v
	}
	return report
}

// Export exports the meta-learner state.
func (m *MetaLearner) Export() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state := struct {
		Strategies     map[string]*Strategy            `json:"strategies"`
		Performance    map[string]*StrategyPerformance `json:"performance"`
		TaskStrategies map[string]string               `json:"task_strategies"`
		ActiveStrategy string                          `json:"active_strategy"`
	}{
		Strategies:     m.strategies,
		Performance:    m.performance,
		TaskStrategies: m.taskStrategies,
		ActiveStrategy: m.activeStrategy,
	}

	return json.Marshal(state)
}

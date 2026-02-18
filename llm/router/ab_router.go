package router

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	llmpkg "github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// Compile-time interface check.
var _ llmpkg.Provider = (*ABRouter)(nil)

// ABVariant represents one variant in an A/B test.
type ABVariant struct {
	// Name is the variant identifier (e.g. "control", "experiment_a").
	Name string
	// Provider is the LLM provider used by this variant.
	Provider llmpkg.Provider
	// Weight is the traffic weight (0-100). All variant weights must sum to 100.
	Weight int
	// Metadata holds arbitrary key-value pairs for this variant.
	Metadata map[string]string
}

// ABMetrics collects per-variant request metrics.
type ABMetrics struct {
	VariantName    string
	TotalRequests  int64
	SuccessCount   int64
	FailureCount   int64
	TotalLatencyMs int64
	TotalCost      float64
	QualityScores  []float64
	mu             sync.Mutex
}

// RecordRequest records a single request outcome.
func (m *ABMetrics) RecordRequest(latencyMs int64, cost float64, success bool, qualityScore float64) {
	atomic.AddInt64(&m.TotalRequests, 1)
	atomic.AddInt64(&m.TotalLatencyMs, latencyMs)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalCost += cost
	if success {
		m.SuccessCount++
	} else {
		m.FailureCount++
	}
	if qualityScore > 0 {
		m.QualityScores = append(m.QualityScores, qualityScore)
	}
}

// GetAvgLatencyMs returns the average latency in milliseconds.
func (m *ABMetrics) GetAvgLatencyMs() float64 {
	total := atomic.LoadInt64(&m.TotalRequests)
	if total == 0 {
		return 0
	}
	return float64(atomic.LoadInt64(&m.TotalLatencyMs)) / float64(total)
}

// GetSuccessRate returns the success rate as a value between 0 and 1.
func (m *ABMetrics) GetSuccessRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := m.SuccessCount + m.FailureCount
	if total == 0 {
		return 0
	}
	return float64(m.SuccessCount) / float64(total)
}

// GetAvgQualityScore returns the average quality score.
func (m *ABMetrics) GetAvgQualityScore() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.QualityScores) == 0 {
		return 0
	}
	var sum float64
	for _, s := range m.QualityScores {
		sum += s
	}
	return sum / float64(len(m.QualityScores))
}

// ABTestConfig holds the configuration for an A/B test.
type ABTestConfig struct {
	// Name identifies this test.
	Name string
	// Variants lists the test variants.
	Variants []ABVariant
	// StickyRouting enables deterministic routing for the same user/session.
	StickyRouting bool
	// StickyKey selects which request field to use: "user_id", "session_id", or "tenant_id".
	StickyKey string
	// StartTime is when the test begins.
	StartTime time.Time
	// EndTime is when the test ends (zero value means indefinite).
	EndTime time.Time
}

// ABRouter is an A/B test router that implements llmpkg.Provider.
type ABRouter struct {
	config  ABTestConfig
	metrics map[string]*ABMetrics // variantName -> metrics

	// Sticky routing cache.
	stickyCache   map[string]string // stickyKey -> variantName
	stickyCacheMu sync.RWMutex

	// Dynamic weight adjustment.
	dynamicWeights map[string]int // variantName -> weight
	weightsMu      sync.RWMutex

	logger *zap.Logger
	rng    *rand.Rand
	rngMu  sync.Mutex
}

// NewABRouter creates a new A/B test router.
func NewABRouter(config ABTestConfig, logger *zap.Logger) (*ABRouter, error) {
	if len(config.Variants) < 2 {
		return nil, fmt.Errorf("A/B test requires at least 2 variants")
	}

	totalWeight := 0
	for _, v := range config.Variants {
		totalWeight += v.Weight
	}
	if totalWeight != 100 {
		return nil, fmt.Errorf("variant weights must sum to 100, got %d", totalWeight)
	}

	metrics := make(map[string]*ABMetrics)
	dynamicWeights := make(map[string]int)
	for _, v := range config.Variants {
		metrics[v.Name] = &ABMetrics{VariantName: v.Name}
		dynamicWeights[v.Name] = v.Weight
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &ABRouter{
		config:         config,
		metrics:        metrics,
		stickyCache:    make(map[string]string),
		dynamicWeights: dynamicWeights,
		logger:         logger.With(zap.String("component", "ab_router")),
		rng:            rand.New(rand.NewSource(time.Now().UnixNano())),
	}, nil
}

// selectVariant picks a variant for the given request.
func (r *ABRouter) selectVariant(ctx context.Context, req *llmpkg.ChatRequest) (*ABVariant, error) {
	// 1. Check if the test is still active.
	now := time.Now()
	if !r.config.EndTime.IsZero() && now.After(r.config.EndTime) {
		return &r.config.Variants[0], nil
	}

	// 2. Sticky routing.
	if r.config.StickyRouting {
		stickyKey := r.extractStickyKey(req)
		if stickyKey != "" {
			r.stickyCacheMu.RLock()
			variantName, exists := r.stickyCache[stickyKey]
			r.stickyCacheMu.RUnlock()

			if exists {
				for i := range r.config.Variants {
					if r.config.Variants[i].Name == variantName {
						return &r.config.Variants[i], nil
					}
				}
			}

			// First request for this key -- use deterministic hash.
			variant := r.hashBasedSelect(stickyKey)
			r.stickyCacheMu.Lock()
			r.stickyCache[stickyKey] = variant.Name
			r.stickyCacheMu.Unlock()
			return variant, nil
		}
	}

	// 3. Weighted random selection.
	return r.weightedRandomSelect(), nil
}

func (r *ABRouter) extractStickyKey(req *llmpkg.ChatRequest) string {
	switch r.config.StickyKey {
	case "user_id":
		return req.UserID
	case "session_id":
		return req.TraceID
	case "tenant_id":
		return req.TenantID
	default:
		return req.UserID
	}
}

func (r *ABRouter) hashBasedSelect(key string) *ABVariant {
	h := sha256.Sum256([]byte(key))
	hashVal := binary.BigEndian.Uint64(h[:8])
	bucket := int(hashVal % 100)

	r.weightsMu.RLock()
	defer r.weightsMu.RUnlock()

	cumulative := 0
	for i := range r.config.Variants {
		w := r.dynamicWeights[r.config.Variants[i].Name]
		cumulative += w
		if bucket < cumulative {
			return &r.config.Variants[i]
		}
	}
	return &r.config.Variants[0]
}

func (r *ABRouter) weightedRandomSelect() *ABVariant {
	r.weightsMu.RLock()
	defer r.weightsMu.RUnlock()

	r.rngMu.Lock()
	target := r.rng.Intn(100)
	r.rngMu.Unlock()

	cumulative := 0
	for i := range r.config.Variants {
		w := r.dynamicWeights[r.config.Variants[i].Name]
		cumulative += w
		if target < cumulative {
			return &r.config.Variants[i]
		}
	}
	return &r.config.Variants[0]
}

// Completion implements llmpkg.Provider.
func (r *ABRouter) Completion(ctx context.Context, req *llmpkg.ChatRequest) (*llmpkg.ChatResponse, error) {
	variant, err := r.selectVariant(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ab_router: select variant failed: %w", err)
	}

	r.logger.Debug("routing request to variant",
		zap.String("variant", variant.Name),
		zap.String("test", r.config.Name))

	start := time.Now()
	resp, err := variant.Provider.Completion(ctx, req)
	latencyMs := time.Since(start).Milliseconds()

	metrics := r.metrics[variant.Name]
	cost := 0.0
	if resp != nil {
		cost = float64(resp.Usage.TotalTokens) * 0.00001
	}
	metrics.RecordRequest(latencyMs, cost, err == nil, 0)

	if resp != nil {
		resp.Provider = fmt.Sprintf("%s[%s]", resp.Provider, variant.Name)
	}

	return resp, err
}

// Stream implements llmpkg.Provider.
func (r *ABRouter) Stream(ctx context.Context, req *llmpkg.ChatRequest) (<-chan llmpkg.StreamChunk, error) {
	variant, err := r.selectVariant(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ab_router: select variant failed: %w", err)
	}

	r.logger.Debug("streaming request to variant",
		zap.String("variant", variant.Name))

	return variant.Provider.Stream(ctx, req)
}

// HealthCheck implements llmpkg.Provider. All variants must be healthy.
func (r *ABRouter) HealthCheck(ctx context.Context) (*llmpkg.HealthStatus, error) {
	for _, v := range r.config.Variants {
		status, err := v.Provider.HealthCheck(ctx)
		if err != nil || !status.Healthy {
			return &llmpkg.HealthStatus{Healthy: false}, err
		}
	}
	return &llmpkg.HealthStatus{Healthy: true}, nil
}

// Name implements llmpkg.Provider.
func (r *ABRouter) Name() string {
	return fmt.Sprintf("ab_router[%s]", r.config.Name)
}

// SupportsNativeFunctionCalling implements llmpkg.Provider.
// Returns true only if all variants support it.
func (r *ABRouter) SupportsNativeFunctionCalling() bool {
	for _, v := range r.config.Variants {
		if !v.Provider.SupportsNativeFunctionCalling() {
			return false
		}
	}
	return true
}

// UpdateWeights dynamically adjusts variant weights. Weights must sum to 100.
func (r *ABRouter) UpdateWeights(weights map[string]int) error {
	total := 0
	for _, w := range weights {
		total += w
	}
	if total != 100 {
		return fmt.Errorf("weights must sum to 100, got %d", total)
	}

	r.weightsMu.Lock()
	defer r.weightsMu.Unlock()

	for name, w := range weights {
		r.dynamicWeights[name] = w
	}

	// Clear sticky cache so new weights take effect.
	if r.config.StickyRouting {
		r.stickyCacheMu.Lock()
		r.stickyCache = make(map[string]string)
		r.stickyCacheMu.Unlock()
	}

	r.logger.Info("A/B test weights updated",
		zap.String("test", r.config.Name),
		zap.Any("weights", weights))

	return nil
}

// GetMetrics returns per-variant metrics.
func (r *ABRouter) GetMetrics() map[string]*ABMetrics {
	return r.metrics
}

// GetReport returns a summary report for all variants.
func (r *ABRouter) GetReport() map[string]map[string]interface{} {
	report := make(map[string]map[string]interface{})
	for name, m := range r.metrics {
		report[name] = map[string]interface{}{
			"total_requests":    atomic.LoadInt64(&m.TotalRequests),
			"success_rate":      m.GetSuccessRate(),
			"avg_latency_ms":    m.GetAvgLatencyMs(),
			"avg_quality_score": m.GetAvgQualityScore(),
			"total_cost":        m.TotalCost,
		}
	}
	return report
}

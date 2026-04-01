package channelstore

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

// RuntimeMetrics tracks recent call outcomes for a single channel or key.
type RuntimeMetrics struct {
	TotalCalls     int64
	SuccessCount   int64
	RateLimitCount int64
	FailureCount   int64
	LastUpdated    time.Time
}

// MetricsSource provides runtime metrics for adaptive weight computation.
// Implementations may store metrics in-memory, Redis, or a database.
type MetricsSource interface {
	GetMetrics(ctx context.Context, channelID string) (*RuntimeMetrics, error)
}

// AdaptiveWeightConfig controls the adaptive weight computation algorithm.
type AdaptiveWeightConfig struct {
	// SuccessBoost is the weight multiplier bonus for high success rates.
	// Default: 0.8
	SuccessBoost float64

	// RateLimitPenalty is the weight multiplier penalty for 429 rate limits.
	// Default: 1.5
	RateLimitPenalty float64

	// FailurePenalty is the weight multiplier penalty for non-rate-limit failures.
	// Default: 0.6
	FailurePenalty float64

	// BaseFactor is the starting factor before applying success/failure adjustments.
	// Default: 0.6
	BaseFactor float64

	// MinFactor clamps the minimum factor to prevent a channel from being
	// completely excluded.
	// Default: 0.2
	MinFactor float64

	// MaxFactor clamps the maximum factor to prevent runaway weight inflation.
	// Default: 3.0
	MaxFactor float64

	// MinCalls is the minimum number of calls before adaptive weighting kicks in.
	// Below this threshold the base weight is used as-is.
	// Default: 5
	MinCalls int64
}

func (c AdaptiveWeightConfig) withDefaults() AdaptiveWeightConfig {
	if c.SuccessBoost == 0 {
		c.SuccessBoost = 0.8
	}
	if c.RateLimitPenalty == 0 {
		c.RateLimitPenalty = 1.5
	}
	if c.FailurePenalty == 0 {
		c.FailurePenalty = 0.6
	}
	if c.BaseFactor == 0 {
		c.BaseFactor = 0.6
	}
	if c.MinFactor == 0 {
		c.MinFactor = 0.2
	}
	if c.MaxFactor == 0 {
		c.MaxFactor = 3.0
	}
	if c.MinCalls == 0 {
		c.MinCalls = 5
	}
	return c
}

// AdaptiveWeightedSelector extends PriorityWeightedSelector with runtime
// metrics-based dynamic weight adjustment. Channels with higher success rates
// receive higher effective weights; channels with high 429/failure rates are
// penalized.
type AdaptiveWeightedSelector struct {
	Source                Store
	Metrics               MetricsSource
	Config                AdaptiveWeightConfig
	AllowKeylessSelection bool

	mu  sync.Mutex
	rng *rand.Rand
}

var _ router.ChannelSelector = (*AdaptiveWeightedSelector)(nil)

// AdaptiveSelectorOptions configures an AdaptiveWeightedSelector.
type AdaptiveSelectorOptions struct {
	Random  *rand.Rand
	Config  AdaptiveWeightConfig
	Metrics MetricsSource
}

// NewAdaptiveWeightedSelector creates a selector that dynamically adjusts
// candidate weights based on runtime call metrics.
func NewAdaptiveWeightedSelector(source Store, metrics MetricsSource, opts AdaptiveSelectorOptions) *AdaptiveWeightedSelector {
	sel := &AdaptiveWeightedSelector{
		Source:  source,
		Metrics: metrics,
		Config:  opts.Config.withDefaults(),
	}
	if opts.Random != nil {
		sel.rng = opts.Random
	}
	return sel
}

func (s *AdaptiveWeightedSelector) SelectChannel(
	ctx context.Context,
	request *router.ChannelRouteRequest,
	resolution *router.ModelResolution,
	mappings []router.ChannelModelMapping,
) (*router.ChannelSelection, error) {
	if s == nil || s.Source == nil {
		return nil, errNilStore
	}

	// Delegate base candidate building to the priority-weighted logic
	base := &PriorityWeightedSelector{
		Source:                s.Source,
		AllowKeylessSelection: s.AllowKeylessSelection,
	}
	// We need to intercept after candidates are built but before selection.
	// Since PriorityWeightedSelector doesn't expose internals, we replicate
	// the core flow here with adaptive weight injection.

	providerHint := firstNonEmpty(providerHintFromResolution(resolution), providerHintFromRequest(request))
	region := firstNonEmpty(regionFromResolution(resolution), regionFromRequest(request))
	excludedChannels := excludedChannelsFromRequest(request)
	excludedKeys := excludedKeysFromRequest(request)
	_ = base // suppress unused

	filteredMappings := filterMappings(mappings, providerHint, region, excludedChannels)
	if len(filteredMappings) == 0 {
		return nil, nil
	}

	channelIndex, err := s.loadChannels(ctx, filteredMappings)
	if err != nil {
		return nil, err
	}

	candidates := make([]selectionCandidate, 0, len(filteredMappings))
	for _, mapping := range filteredMappings {
		channelID := strings.TrimSpace(mapping.ChannelID)
		if channelID == "" {
			continue
		}
		channel := channelIndex[channelID]

		keys, err := s.Source.ListKeys(ctx, channelID)
		if err != nil {
			return nil, err
		}

		if len(keys) == 0 && s.AllowKeylessSelection {
			provider := firstNonEmpty(strings.TrimSpace(mapping.Provider), strings.TrimSpace(channel.Provider))
			candidateRegion := firstNonEmpty(strings.TrimSpace(mapping.Region), strings.TrimSpace(channel.Region))
			if !matchesProvider(provider, providerHint) || !matchesRegion(candidateRegion, region) {
				continue
			}
			priority := resolvePriority(mapping.Priority, channel.Priority)
			weight := resolveWeight(mapping.Weight, channel.Weight)
			candidates = append(candidates, selectionCandidate{
				selection: router.ChannelSelection{
					MappingID:   strings.TrimSpace(mapping.MappingID),
					ChannelID:   channelID,
					Provider:    provider,
					RemoteModel: firstNonEmpty(strings.TrimSpace(mapping.RemoteModel), resolvedModelFromResolution(resolution), requestedModelFrom(request)),
					BaseURL:     firstNonEmpty(strings.TrimSpace(mapping.BaseURL), strings.TrimSpace(channel.BaseURL)),
					Region:      candidateRegion,
					Priority:    priority,
					Weight:      normalizedWeight(weight),
					Metadata:    mergeStringMaps(channel.Metadata, mapping.Metadata),
				},
				weight: normalizedWeight(weight),
			})
			continue
		}

		for _, key := range keys {
			candidate, ok := buildCandidate(mapping, channel, key, request, resolution, excludedChannels, excludedKeys, providerHint, region)
			if !ok {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// Apply adaptive weights
	if s.Metrics != nil {
		s.applyAdaptiveWeights(ctx, candidates)
	}

	// Priority tier selection
	bestPriority := normalizedPriority(candidates[0].selection.Priority)
	for _, c := range candidates[1:] {
		if p := normalizedPriority(c.selection.Priority); p < bestPriority {
			bestPriority = p
		}
	}
	tier := make([]selectionCandidate, 0, len(candidates))
	for _, c := range candidates {
		if normalizedPriority(c.selection.Priority) == bestPriority {
			tier = append(tier, c)
		}
	}

	chosen := chooseWeighted(tier, s.drawIndex)
	selection := chosen.selection
	return &selection, nil
}

func (s *AdaptiveWeightedSelector) applyAdaptiveWeights(ctx context.Context, candidates []selectionCandidate) {
	cfg := s.Config
	for i := range candidates {
		channelID := candidates[i].selection.ChannelID
		if channelID == "" {
			continue
		}
		metrics, err := s.Metrics.GetMetrics(ctx, channelID)
		if err != nil || metrics == nil || metrics.TotalCalls < cfg.MinCalls {
			continue
		}
		candidates[i].weight = computeAdaptiveWeight(candidates[i].weight, metrics, cfg)
	}
}

func (s *AdaptiveWeightedSelector) loadChannels(ctx context.Context, mappings []router.ChannelModelMapping) (map[string]Channel, error) {
	channels, err := s.Source.GetChannels(ctx, mappingChannelIDs(mappings))
	if err != nil {
		return nil, err
	}
	index := make(map[string]Channel, len(channels))
	for _, ch := range channels {
		id := strings.TrimSpace(ch.ID)
		if id != "" {
			index[id] = ch
		}
	}
	return index, nil
}

func (s *AdaptiveWeightedSelector) drawIndex(totalWeight int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rng == nil {
		s.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return s.rng.Intn(totalWeight)
}

// computeAdaptiveWeight adjusts base weight using runtime metrics.
//
// Algorithm:
//
//	factor = BaseFactor + successRate * SuccessBoost
//	factor -= rateLimitRate * RateLimitPenalty
//	factor -= nonRateFailureRate * FailurePenalty
//	factor = clamp(factor, MinFactor, MaxFactor)
//	adaptedWeight = round(baseWeight * factor)
func computeAdaptiveWeight(baseWeight int, metrics *RuntimeMetrics, cfg AdaptiveWeightConfig) int {
	if metrics == nil || metrics.TotalCalls == 0 {
		return normalizedWeight(baseWeight)
	}

	total := float64(metrics.TotalCalls)
	successRate := float64(metrics.SuccessCount) / total
	rateLimitRate := float64(metrics.RateLimitCount) / total
	nonRateFailure := float64(metrics.FailureCount-metrics.RateLimitCount) / total
	if nonRateFailure < 0 {
		nonRateFailure = 0
	}

	factor := cfg.BaseFactor + successRate*cfg.SuccessBoost
	factor -= rateLimitRate * cfg.RateLimitPenalty
	factor -= nonRateFailure * cfg.FailurePenalty

	if factor < cfg.MinFactor {
		factor = cfg.MinFactor
	}
	if factor > cfg.MaxFactor {
		factor = cfg.MaxFactor
	}

	weight := int(math.Round(float64(normalizedWeight(baseWeight)) * factor))
	if weight < 1 {
		return 1
	}
	return weight
}

// InMemoryMetricsSource is a thread-safe in-memory MetricsSource.
type InMemoryMetricsSource struct {
	mu      sync.RWMutex
	metrics map[string]*RuntimeMetrics
}

func NewInMemoryMetricsSource() *InMemoryMetricsSource {
	return &InMemoryMetricsSource{metrics: make(map[string]*RuntimeMetrics)}
}

func (m *InMemoryMetricsSource) GetMetrics(_ context.Context, channelID string) (*RuntimeMetrics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rm, ok := m.metrics[channelID]
	if !ok {
		return nil, nil
	}
	cp := *rm
	return &cp, nil
}

// Record records a call outcome for a channel.
func (m *InMemoryMetricsSource) Record(channelID string, success bool, isRateLimit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rm, ok := m.metrics[channelID]
	if !ok {
		rm = &RuntimeMetrics{}
		m.metrics[channelID] = rm
	}
	rm.TotalCalls++
	if success {
		rm.SuccessCount++
	} else {
		rm.FailureCount++
		if isRateLimit {
			rm.RateLimitCount++
		}
	}
	rm.LastUpdated = time.Now()
}

// Reset clears all metrics.
func (m *InMemoryMetricsSource) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = make(map[string]*RuntimeMetrics)
}

var errNilStore = fmt.Errorf("channelstore selector requires a store source")

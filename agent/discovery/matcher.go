package discovery

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CapabilityMatcher is the default implementation of the Matcher interface.
// It provides semantic matching, capability scoring, and load balancing.
type CapabilityMatcher struct {
	registry Registry
	config   *MatcherConfig
	logger   *zap.Logger

	// roundRobinIndex tracks the current index for round-robin selection.
	roundRobinIndex map[string]int
	rrMu            sync.Mutex

	// random source for random selection.
	rng *rand.Rand
}

// MatcherConfig holds configuration for the capability matcher.
type MatcherConfig struct {
	// DefaultStrategy is the default matching strategy.
	DefaultStrategy MatchStrategy `json:"default_strategy"`

	// DefaultLimit is the default limit for match results.
	DefaultLimit int `json:"default_limit"`

	// DefaultTimeout is the default timeout for match operations.
	DefaultTimeout time.Duration `json:"default_timeout"`

	// MinScoreThreshold is the minimum score threshold for matches.
	MinScoreThreshold float64 `json:"min_score_threshold"`

	// LoadWeight is the weight for load in scoring (0-1).
	LoadWeight float64 `json:"load_weight"`

	// ScoreWeight is the weight for capability score in scoring (0-1).
	ScoreWeight float64 `json:"score_weight"`

	// LatencyWeight is the weight for latency in scoring (0-1).
	LatencyWeight float64 `json:"latency_weight"`

	// EnableSemanticMatching enables semantic matching for task descriptions.
	EnableSemanticMatching bool `json:"enable_semantic_matching"`

	// SemanticSimilarityThreshold is the threshold for semantic similarity.
	SemanticSimilarityThreshold float64 `json:"semantic_similarity_threshold"`
}

// DefaultMatcherConfig returns a MatcherConfig with sensible defaults.
func DefaultMatcherConfig() *MatcherConfig {
	return &MatcherConfig{
		DefaultStrategy:             MatchStrategyBestMatch,
		DefaultLimit:                10,
		DefaultTimeout:              5 * time.Second,
		MinScoreThreshold:           0.0,
		LoadWeight:                  0.3,
		ScoreWeight:                 0.5,
		LatencyWeight:               0.2,
		EnableSemanticMatching:      true,
		SemanticSimilarityThreshold: 0.5,
	}
}

// NewCapabilityMatcher creates a new capability matcher.
func NewCapabilityMatcher(registry Registry, config *MatcherConfig, logger *zap.Logger) *CapabilityMatcher {
	if config == nil {
		config = DefaultMatcherConfig()
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	return &CapabilityMatcher{
		registry:        registry,
		config:          config,
		logger:          logger.With(zap.String("component", "capability_matcher")),
		roundRobinIndex: make(map[string]int),
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Match finds agents matching the given request.
func (m *CapabilityMatcher) Match(ctx context.Context, req *MatchRequest) ([]*MatchResult, error) {
	if req == nil {
		return nil, fmt.Errorf("match request is nil")
	}

	// Apply defaults
	if req.Strategy == "" {
		req.Strategy = m.config.DefaultStrategy
	}
	if req.Limit <= 0 {
		req.Limit = m.config.DefaultLimit
	}
	if req.Timeout <= 0 {
		req.Timeout = m.config.DefaultTimeout
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Get all agents
	agents, err := m.registry.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	// Filter and score agents
	results := make([]*MatchResult, 0)
	for _, agent := range agents {
		// Skip excluded agents
		if m.isExcluded(agent.Card.Name, req.ExcludedAgents) {
			continue
		}

		// Skip offline agents
		if agent.Status != AgentStatusOnline {
			continue
		}

		// Check load constraint
		if req.MaxLoad > 0 && agent.Load > req.MaxLoad {
			continue
		}

		// Calculate match score
		score, matchedCaps, confidence, reason := m.calculateMatchScore(ctx, agent, req)

		// Skip if below threshold
		if score < req.MinScore && score < m.config.MinScoreThreshold {
			continue
		}

		// Skip if no capabilities matched
		if len(matchedCaps) == 0 && len(req.RequiredCapabilities) > 0 {
			continue
		}

		results = append(results, &MatchResult{
			Agent:               agent,
			MatchedCapabilities: matchedCaps,
			Score:               score,
			Confidence:          confidence,
			Reason:              reason,
		})
	}

	// Sort results based on strategy
	m.sortResults(results, req.Strategy)

	// Apply limit
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	m.logger.Debug("match completed",
		zap.Int("results", len(results)),
		zap.String("strategy", string(req.Strategy)),
	)

	return results, nil
}

// MatchOne finds the best matching agent for the given request.
func (m *CapabilityMatcher) MatchOne(ctx context.Context, req *MatchRequest) (*MatchResult, error) {
	req.Limit = 1
	results, err := m.Match(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no matching agent found")
	}

	return results[0], nil
}

// Score calculates the match score for an agent against a request.
func (m *CapabilityMatcher) Score(ctx context.Context, agent *AgentInfo, req *MatchRequest) (float64, error) {
	if agent == nil || req == nil {
		return 0, fmt.Errorf("agent or request is nil")
	}

	score, _, _, _ := m.calculateMatchScore(ctx, agent, req)
	return score, nil
}

// calculateMatchScore calculates the match score for an agent.
func (m *CapabilityMatcher) calculateMatchScore(ctx context.Context, agent *AgentInfo, req *MatchRequest) (float64, []CapabilityInfo, float64, string) {
	var matchedCaps []CapabilityInfo
	var reasons []string
	var totalScore float64
	var confidence float64 = 1.0

	// 1. Check required capabilities
	requiredMatched := 0
	for _, reqCap := range req.RequiredCapabilities {
		for _, agentCap := range agent.Capabilities {
			if m.capabilityMatches(agentCap.Capability.Name, reqCap) {
				matchedCaps = append(matchedCaps, agentCap)
				requiredMatched++
				break
			}
		}
	}

	if len(req.RequiredCapabilities) > 0 {
		if requiredMatched < len(req.RequiredCapabilities) {
			// Not all required capabilities matched
			return 0, nil, 0, "missing required capabilities"
		}
		totalScore += 40.0 // Base score for matching all required capabilities
		reasons = append(reasons, fmt.Sprintf("matched %d required capabilities", requiredMatched))
	}

	// 2. Check preferred capabilities
	preferredMatched := 0
	for _, prefCap := range req.PreferredCapabilities {
		for _, agentCap := range agent.Capabilities {
			if m.capabilityMatches(agentCap.Capability.Name, prefCap) {
				// Only add if not already in matchedCaps
				found := false
				for _, mc := range matchedCaps {
					if mc.Capability.Name == agentCap.Capability.Name {
						found = true
						break
					}
				}
				if !found {
					matchedCaps = append(matchedCaps, agentCap)
				}
				preferredMatched++
				break
			}
		}
	}

	if len(req.PreferredCapabilities) > 0 {
		preferredRatio := float64(preferredMatched) / float64(len(req.PreferredCapabilities))
		totalScore += preferredRatio * 20.0
		reasons = append(reasons, fmt.Sprintf("matched %d/%d preferred capabilities", preferredMatched, len(req.PreferredCapabilities)))
	}

	// 3. Check required tags
	if len(req.RequiredTags) > 0 {
		tagMatched := 0
		for _, reqTag := range req.RequiredTags {
			for _, agentCap := range agent.Capabilities {
				for _, tag := range agentCap.Tags {
					if strings.EqualFold(tag, reqTag) {
						tagMatched++
						break
					}
				}
			}
		}
		if tagMatched < len(req.RequiredTags) {
			return 0, nil, 0, "missing required tags"
		}
		totalScore += 10.0
		reasons = append(reasons, fmt.Sprintf("matched %d required tags", tagMatched))
	}

	// 4. Semantic matching for task description
	if m.config.EnableSemanticMatching && req.TaskDescription != "" {
		semanticScore, semanticConfidence := m.calculateSemanticScore(agent, req.TaskDescription)
		if semanticScore > m.config.SemanticSimilarityThreshold {
			totalScore += semanticScore * 20.0
			confidence *= semanticConfidence
			reasons = append(reasons, fmt.Sprintf("semantic match: %.2f", semanticScore))
		}
	}

	// 5. Calculate capability-based score
	if len(matchedCaps) > 0 {
		var capScore float64
		for _, cap := range matchedCaps {
			capScore += cap.Score
		}
		avgCapScore := capScore / float64(len(matchedCaps))
		totalScore += (avgCapScore / 100.0) * m.config.ScoreWeight * 10.0
	}

	// 6. Apply load penalty
	loadPenalty := agent.Load * m.config.LoadWeight * 10.0
	totalScore -= loadPenalty

	// 7. Apply latency penalty
	if len(matchedCaps) > 0 {
		var avgLatency time.Duration
		for _, cap := range matchedCaps {
			avgLatency += cap.AvgLatency
		}
		avgLatency /= time.Duration(len(matchedCaps))
		// Normalize latency (assume 1s is baseline)
		latencyPenalty := float64(avgLatency) / float64(time.Second) * m.config.LatencyWeight * 5.0
		totalScore -= latencyPenalty
	}

	// Normalize score to 0-100
	totalScore = math.Max(0, math.Min(100, totalScore))

	reason := strings.Join(reasons, "; ")
	return totalScore, matchedCaps, confidence, reason
}

// capabilityMatches checks if a capability name matches a required capability.
func (m *CapabilityMatcher) capabilityMatches(capName, required string) bool {
	// Exact match
	if strings.EqualFold(capName, required) {
		return true
	}

	// Prefix match (e.g., "code_review" matches "code_review_python")
	if strings.HasPrefix(strings.ToLower(capName), strings.ToLower(required)) {
		return true
	}

	// Contains match
	if strings.Contains(strings.ToLower(capName), strings.ToLower(required)) {
		return true
	}

	return false
}

// calculateSemanticScore calculates semantic similarity between agent capabilities and task description.
func (m *CapabilityMatcher) calculateSemanticScore(agent *AgentInfo, taskDescription string) (float64, float64) {
	// Simple keyword-based semantic matching
	// In production, this would use embeddings or an LLM
	taskWords := m.tokenize(taskDescription)
	if len(taskWords) == 0 {
		return 0, 0
	}

	var totalScore float64
	var matchCount int

	// Check agent description
	agentWords := m.tokenize(agent.Card.Description)
	for _, tw := range taskWords {
		for _, aw := range agentWords {
			if strings.EqualFold(tw, aw) {
				matchCount++
				break
			}
		}
	}

	// Check capability descriptions
	for _, cap := range agent.Capabilities {
		capWords := m.tokenize(cap.Capability.Description)
		for _, tw := range taskWords {
			for _, cw := range capWords {
				if strings.EqualFold(tw, cw) {
					matchCount++
					break
				}
			}
		}
	}

	if matchCount > 0 {
		totalScore = float64(matchCount) / float64(len(taskWords))
		totalScore = math.Min(1.0, totalScore)
	}

	// Confidence is based on how many words matched
	confidence := math.Min(1.0, float64(matchCount)/5.0)

	return totalScore, confidence
}

// tokenize splits text into words for matching.
func (m *CapabilityMatcher) tokenize(text string) []string {
	// Simple tokenization - split on whitespace and punctuation
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})

	// Filter out common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "as": true, "at": true, "by": true, "from": true,
		"this": true, "that": true, "it": true, "its": true,
	}

	filtered := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) > 2 && !stopWords[w] {
			filtered = append(filtered, w)
		}
	}

	return filtered
}

// sortResults sorts match results based on the strategy.
func (m *CapabilityMatcher) sortResults(results []*MatchResult, strategy MatchStrategy) {
	switch strategy {
	case MatchStrategyBestMatch:
		// Sort by score descending
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

	case MatchStrategyLeastLoaded:
		// Sort by load ascending, then by score descending
		sort.Slice(results, func(i, j int) bool {
			if results[i].Agent.Load != results[j].Agent.Load {
				return results[i].Agent.Load < results[j].Agent.Load
			}
			return results[i].Score > results[j].Score
		})

	case MatchStrategyHighestScore:
		// Sort by capability score descending
		sort.Slice(results, func(i, j int) bool {
			var scoreI, scoreJ float64
			for _, cap := range results[i].MatchedCapabilities {
				scoreI += cap.Score
			}
			for _, cap := range results[j].MatchedCapabilities {
				scoreJ += cap.Score
			}
			return scoreI > scoreJ
		})

	case MatchStrategyRoundRobin:
		// Shuffle results for round-robin effect
		m.rng.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})

	case MatchStrategyRandom:
		// Shuffle results randomly
		m.rng.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}
}

// isExcluded checks if an agent ID is in the excluded list.
func (m *CapabilityMatcher) isExcluded(agentID string, excluded []string) bool {
	for _, ex := range excluded {
		if ex == agentID {
			return true
		}
	}
	return false
}

// GetNextRoundRobin returns the next agent in round-robin order for a given capability.
func (m *CapabilityMatcher) GetNextRoundRobin(ctx context.Context, capabilityName string) (*AgentInfo, error) {
	caps, err := m.registry.FindCapabilities(ctx, capabilityName)
	if err != nil {
		return nil, err
	}

	if len(caps) == 0 {
		return nil, fmt.Errorf("no agents found with capability %s", capabilityName)
	}

	m.rrMu.Lock()
	defer m.rrMu.Unlock()

	idx := m.roundRobinIndex[capabilityName]
	m.roundRobinIndex[capabilityName] = (idx + 1) % len(caps)

	agentID := caps[idx].AgentID
	return m.registry.GetAgent(ctx, agentID)
}

// FindBestAgent finds the best agent for a task using the default strategy.
func (m *CapabilityMatcher) FindBestAgent(ctx context.Context, taskDescription string, requiredCapabilities []string) (*AgentInfo, error) {
	result, err := m.MatchOne(ctx, &MatchRequest{
		TaskDescription:      taskDescription,
		RequiredCapabilities: requiredCapabilities,
		Strategy:             MatchStrategyBestMatch,
	})
	if err != nil {
		return nil, err
	}
	return result.Agent, nil
}

// FindLeastLoadedAgent finds the least loaded agent with the required capabilities.
func (m *CapabilityMatcher) FindLeastLoadedAgent(ctx context.Context, requiredCapabilities []string) (*AgentInfo, error) {
	result, err := m.MatchOne(ctx, &MatchRequest{
		RequiredCapabilities: requiredCapabilities,
		Strategy:             MatchStrategyLeastLoaded,
	})
	if err != nil {
		return nil, err
	}
	return result.Agent, nil
}

// Ensure CapabilityMatcher implements Matcher interface.
var _ Matcher = (*CapabilityMatcher)(nil)

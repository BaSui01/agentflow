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

// 能力Matcher是Matcher接口的默认执行.
// 它提供语义匹配,能力评分,和负载平衡.
type CapabilityMatcher struct {
	registry Registry
	config   *MatcherConfig
	logger   *zap.Logger

	// roundRobinIndex 跟踪当前用于回合-robin选择的索引。
	roundRobinIndex map[string]int
	rrMu            sync.Mutex

	// 随机选择源。
	rng *rand.Rand
}

// MatcherConfig持有能力匹配器的配置.
type MatcherConfig struct {
	// 默认策略是默认匹配策略.
	DefaultStrategy MatchStrategy `json:"default_strategy"`

	// 默认限制是匹配结果的默认限制 。
	DefaultLimit int `json:"default_limit"`

	// 默认超时是匹配操作的默认超时.
	DefaultTimeout time.Duration `json:"default_timeout"`

	// MinScore Threershold是比赛的最低得分门槛.
	MinScoreThreshold float64 `json:"min_score_threshold"`

	// 载重是积分(0-1)中载重.
	LoadWeight float64 `json:"load_weight"`

	// 分数(ScoreWight)是分数(0-1)中能力分数的权重.
	ScoreWeight float64 `json:"score_weight"`

	// latencyWight是积分(0-1)中耐久的重量.
	LatencyWeight float64 `json:"latency_weight"`

	// 启用语义匹配可实现语义匹配任务描述.
	EnableSemanticMatching bool `json:"enable_semantic_matching"`

	// 语义相似 阈值是语义相似性的阈值.
	SemanticSimilarityThreshold float64 `json:"semantic_similarity_threshold"`
}

// 默认 MatcherConfig 返回带有合理默认的 MatcherConfig 。
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

// 新能力 Matcher创建了新的能力匹配器.
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

// Match 找到匹配给定请求的代理 。
func (m *CapabilityMatcher) Match(ctx context.Context, req *MatchRequest) ([]*MatchResult, error) {
	if req == nil {
		return nil, fmt.Errorf("match request is nil")
	}

	// 应用默认
	if req.Strategy == "" {
		req.Strategy = m.config.DefaultStrategy
	}
	if req.Limit <= 0 {
		req.Limit = m.config.DefaultLimit
	}
	if req.Timeout <= 0 {
		req.Timeout = m.config.DefaultTimeout
	}

	// 以超时创建上下文
	ctx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// 找到所有探员
	agents, err := m.registry.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	// 过滤和计分代理
	results := make([]*MatchResult, 0)
	for _, agent := range agents {
		// 跳过排除的代理
		if m.isExcluded(agent.Card.Name, req.ExcludedAgents) {
			continue
		}

		// 跳过线下代理
		if agent.Status != AgentStatusOnline {
			continue
		}

		// 检查负载约束
		if req.MaxLoad > 0 && agent.Load > req.MaxLoad {
			continue
		}

		// 计算匹配分数
		score, matchedCaps, confidence, reason := m.calculateMatchScore(ctx, agent, req)

		// 低于阈值时跳过
		if score < req.MinScore && score < m.config.MinScoreThreshold {
			continue
		}

		// 如果没有匹配的能力就跳过
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

	// 根据战略排序结果
	m.sortResults(results, req.Strategy)

	// 应用限制
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}

	m.logger.Debug("match completed",
		zap.Int("results", len(results)),
		zap.String("strategy", string(req.Strategy)),
	)

	return results, nil
}

// MatchOne 找到指定请求的最佳匹配代理 。
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

// 分数根据请求计算代理商的比分。
func (m *CapabilityMatcher) Score(ctx context.Context, agent *AgentInfo, req *MatchRequest) (float64, error) {
	if agent == nil || req == nil {
		return 0, fmt.Errorf("agent or request is nil")
	}

	score, _, _, _ := m.calculateMatchScore(ctx, agent, req)
	return score, nil
}

// 计算 MatchScore 为代理计算匹配分数。
func (m *CapabilityMatcher) calculateMatchScore(ctx context.Context, agent *AgentInfo, req *MatchRequest) (float64, []CapabilityInfo, float64, string) {
	var matchedCaps []CapabilityInfo
	var reasons []string
	var totalScore float64
	var confidence float64 = 1.0

	// 1. 检查所需能力
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
			// 并非所有所需能力匹配
			return 0, nil, 0, "missing required capabilities"
		}
		totalScore += 40.0 // Base score for matching all required capabilities
		reasons = append(reasons, fmt.Sprintf("matched %d required capabilities", requiredMatched))
	}

	// 2. 检查首选能力
	preferredMatched := 0
	for _, prefCap := range req.PreferredCapabilities {
		for _, agentCap := range agent.Capabilities {
			if m.capabilityMatches(agentCap.Capability.Name, prefCap) {
				// 只添加尚未匹配的 Caps
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

	// 3. 检查所需标签
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

	// 4. 任务描述的语义匹配
	if m.config.EnableSemanticMatching && req.TaskDescription != "" {
		semanticScore, semanticConfidence := m.calculateSemanticScore(agent, req.TaskDescription)
		if semanticScore > m.config.SemanticSimilarityThreshold {
			totalScore += semanticScore * 20.0
			confidence *= semanticConfidence
			reasons = append(reasons, fmt.Sprintf("semantic match: %.2f", semanticScore))
		}
	}

	// 5. 计算能力得分
	if len(matchedCaps) > 0 {
		var capScore float64
		for _, cap := range matchedCaps {
			capScore += cap.Score
		}
		avgCapScore := capScore / float64(len(matchedCaps))
		totalScore += (avgCapScore / 100.0) * m.config.ScoreWeight * 10.0
	}

	// 6. 适用负载处罚
	loadPenalty := agent.Load * m.config.LoadWeight * 10.0
	totalScore -= loadPenalty

	// 7. 适用延迟处罚
	if len(matchedCaps) > 0 {
		var avgLatency time.Duration
		for _, cap := range matchedCaps {
			avgLatency += cap.AvgLatency
		}
		avgLatency /= time.Duration(len(matchedCaps))
		// 常态性(假设 1s 为基线)
		latencyPenalty := float64(avgLatency) / float64(time.Second) * m.config.LatencyWeight * 5.0
		totalScore -= latencyPenalty
	}

	// 将分数正常化到0-100
	totalScore = math.Max(0, math.Min(100, totalScore))

	reason := strings.Join(reasons, "; ")
	return totalScore, matchedCaps, confidence, reason
}

// 能力 匹配一个匹配所需能力的能力名称 。
func (m *CapabilityMatcher) capabilityMatches(capName, required string) bool {
	// 准确匹配
	if strings.EqualFold(capName, required) {
		return true
	}

	// 前缀匹配(例如"code review"与"code review python"相匹配)
	if strings.HasPrefix(strings.ToLower(capName), strings.ToLower(required)) {
		return true
	}

	// 包含匹配
	if strings.Contains(strings.ToLower(capName), strings.ToLower(required)) {
		return true
	}

	return false
}

// 计算SemanticScore计算出代理能力和任务描述之间的语义相似性.
func (m *CapabilityMatcher) calculateSemanticScore(agent *AgentInfo, taskDescription string) (float64, float64) {
	// 基于简单关键字的语义匹配
	// 在生产中,将使用嵌入或LLM
	taskWords := m.tokenize(taskDescription)
	if len(taskWords) == 0 {
		return 0, 0
	}

	var totalScore float64
	var matchCount int

	// 检查代理描述
	agentWords := m.tokenize(agent.Card.Description)
	for _, tw := range taskWords {
		for _, aw := range agentWords {
			if strings.EqualFold(tw, aw) {
				matchCount++
				break
			}
		}
	}

	// 检查能力描述
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

	// 自信是建立在几句话匹配的基础上的
	confidence := math.Min(1.0, float64(matchCount)/5.0)

	return totalScore, confidence
}

// 将文本分割成文字进行匹配。
func (m *CapabilityMatcher) tokenize(text string) []string {
	// 简单的符号化 - 在白空和平分
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})

	// 过滤出常见的句子
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

// 排序结果类型匹配基于策略的结果。
func (m *CapabilityMatcher) sortResults(results []*MatchResult, strategy MatchStrategy) {
	switch strategy {
	case MatchStrategyBestMatch:
		// 按分数递减排序
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})

	case MatchStrategyLeastLoaded:
		// 依负载升起排序,再由分数递减排序
		sort.Slice(results, func(i, j int) bool {
			if results[i].Agent.Load != results[j].Agent.Load {
				return results[i].Agent.Load < results[j].Agent.Load
			}
			return results[i].Score > results[j].Score
		})

	case MatchStrategyHighestScore:
		// 按能力分数递减排序
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
		// 轮旋效果的摇摆结果
		m.rng.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})

	case MatchStrategyRandom:
		// 随机打乱结果
		m.rng.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}
}

// isexcused checked 如果被排除在外的名单上有代理ID。
func (m *CapabilityMatcher) isExcluded(agentID string, excluded []string) bool {
	for _, ex := range excluded {
		if ex == agentID {
			return true
		}
	}
	return false
}

// GetNextRound Robin 返回一个给定能力的下一个代理。
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

// FindBestAgent 找到使用默认策略进行任务的最佳代理 。
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

// FindLeastLoaded Agent 找到装入量最小的具有所需能力的代理.
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

// 确保能力Matcher执行Matcher接口.
var _ Matcher = (*CapabilityMatcher)(nil)

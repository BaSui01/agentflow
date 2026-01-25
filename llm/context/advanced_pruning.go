package context

import (
	"math"
	"sort"

	"go.uber.org/zap"
)

// AdvancedPruningStrategy 高级裁剪策略（基于 2025 年研究）
type AdvancedPruningStrategy struct {
	// 重要性评分权重
	RecencyWeight   float64 `json:"recency_weight"`   // 时间新近性权重
	RelevanceWeight float64 `json:"relevance_weight"` // 语义相关性权重
	PositionWeight  float64 `json:"position_weight"`  // 位置权重
	RoleWeight      float64 `json:"role_weight"`      // 角色权重

	// 动态裁剪参数
	GainThreshold     float64 `json:"gain_threshold"`     // 增益阈值 (τ)
	StoppingThreshold float64 `json:"stopping_threshold"` // 停止阈值 (θ)

	// 连续性保护
	PreserveContinuity bool `json:"preserve_continuity"` // 保留连续性
	MinSegmentSize     int  `json:"min_segment_size"`    // 最小片段大小
}

// DefaultAdvancedPruningStrategy 返回默认高级裁剪策略
func DefaultAdvancedPruningStrategy() AdvancedPruningStrategy {
	return AdvancedPruningStrategy{
		RecencyWeight:      0.3,
		RelevanceWeight:    0.5,
		PositionWeight:     0.1,
		RoleWeight:         0.1,
		GainThreshold:      0.6,
		StoppingThreshold:  1.0,
		PreserveContinuity: true,
		MinSegmentSize:     1,
	}
}

// MessageScore 消息评分
type MessageScore struct {
	Index           int     `json:"index"`
	RecencyScore    float64 `json:"recency_score"`
	RelevanceScore  float64 `json:"relevance_score"`
	PositionScore   float64 `json:"position_score"`
	RoleScore       float64 `json:"role_score"`
	TotalScore      float64 `json:"total_score"`
	NormalizedScore float64 `json:"normalized_score"` // Z-score 归一化
}

// Segment 连续片段
type Segment struct {
	StartIndex int       `json:"start_index"`
	EndIndex   int       `json:"end_index"`
	Score      float64   `json:"score"` // 累积得分
	Messages   []Message `json:"messages"`
}

// AdvancedContextPruner 高级上下文裁剪器
type AdvancedContextPruner struct {
	strategy AdvancedPruningStrategy
	logger   *zap.Logger
}

// NewAdvancedContextPruner 创建高级上下文裁剪器
func NewAdvancedContextPruner(strategy AdvancedPruningStrategy, logger *zap.Logger) *AdvancedContextPruner {
	return &AdvancedContextPruner{
		strategy: strategy,
		logger:   logger,
	}
}

// PruneWithScoring 使用评分进行裁剪
func (p *AdvancedContextPruner) PruneWithScoring(msgs []Message, maxTokens int, currentQuery string) ([]Message, error) {
	if len(msgs) == 0 {
		return msgs, nil
	}

	// 1. 计算每条消息的多维评分
	scores := p.scoreMessages(msgs, currentQuery)

	// 2. Z-score 归一化
	scores = p.normalizeScores(scores)

	// 3. 使用 KadaneDial 算法识别高价值片段
	segments := p.kadaneDial(scores, msgs)

	// 4. 选择片段直到达到 token 限制
	selected := p.selectSegments(segments, maxTokens)

	p.logger.Info("advanced pruning completed",
		zap.Int("original", len(msgs)),
		zap.Int("pruned", len(selected)),
		zap.Int("segments", len(segments)))

	return selected, nil
}

// scoreMessages 计算消息评分
func (p *AdvancedContextPruner) scoreMessages(msgs []Message, currentQuery string) []MessageScore {
	scores := make([]MessageScore, len(msgs))
	totalMsgs := float64(len(msgs))

	for i, msg := range msgs {
		score := MessageScore{Index: i}

		// 1. 时间新近性评分（越新越高）
		score.RecencyScore = float64(i) / totalMsgs

		// 2. 语义相关性评分（简化版：基于关键词匹配）
		score.RelevanceScore = p.calculateRelevance(msg.Content, currentQuery)

		// 3. 位置评分（开头和结尾更重要）
		if i < int(totalMsgs*0.1) || i > int(totalMsgs*0.9) {
			score.PositionScore = 1.0
		} else {
			score.PositionScore = 0.5
		}

		// 4. 角色评分
		score.RoleScore = p.getRoleScore(msg.Role)

		// 5. 加权总分
		score.TotalScore = score.RecencyScore*p.strategy.RecencyWeight +
			score.RelevanceScore*p.strategy.RelevanceWeight +
			score.PositionScore*p.strategy.PositionWeight +
			score.RoleScore*p.strategy.RoleWeight

		scores[i] = score
	}

	return scores
}

// calculateRelevance 计算相关性（简化版）
func (p *AdvancedContextPruner) calculateRelevance(content, query string) float64 {
	// 生产环境应使用向量嵌入 + 余弦相似度
	// 这里使用简化的关键词匹配
	if query == "" {
		return 0.5
	}

	// 提取关键词并计算匹配度
	queryWords := extractKeywords(query)
	contentWords := extractKeywords(content)

	if len(queryWords) == 0 {
		return 0.5
	}

	matchCount := 0
	for _, qw := range queryWords {
		for _, cw := range contentWords {
			if qw == cw {
				matchCount++
				break
			}
		}
	}

	return float64(matchCount) / float64(len(queryWords))
}

// getRoleScore 获取角色评分
func (p *AdvancedContextPruner) getRoleScore(role Role) float64 {
	switch role {
	case RoleSystem:
		return 1.0
	case RoleUser, RoleAssistant:
		return 0.8
	case RoleTool:
		return 0.5
	default:
		return 0.3
	}
}

// normalizeScores Z-score 归一化
func (p *AdvancedContextPruner) normalizeScores(scores []MessageScore) []MessageScore {
	if len(scores) == 0 {
		return scores
	}

	// 计算均值和标准差
	var sum, sumSq float64
	for _, s := range scores {
		sum += s.TotalScore
		sumSq += s.TotalScore * s.TotalScore
	}

	mean := sum / float64(len(scores))
	variance := (sumSq / float64(len(scores))) - (mean * mean)
	stdDev := math.Sqrt(variance)

	if stdDev == 0 {
		stdDev = 1.0 // 避免除零
	}

	// Z-score 归一化
	for i := range scores {
		scores[i].NormalizedScore = (scores[i].TotalScore - mean) / stdDev
	}

	return scores
}

// kadaneDial KadaneDial 算法（基于 DyCP 论文）
// 识别连续的高价值片段
func (p *AdvancedContextPruner) kadaneDial(scores []MessageScore, msgs []Message) []Segment {
	if len(scores) == 0 {
		return nil
	}

	segments := []Segment{}
	used := make([]bool, len(scores))

	// 迭代查找所有高价值片段
	for {
		// 应用增益阈值
		adjustedScores := make([]float64, len(scores))
		for i, s := range scores {
			if used[i] {
				adjustedScores[i] = -math.MaxFloat64 // 已使用的标记为极小值
			} else {
				adjustedScores[i] = s.NormalizedScore - p.strategy.GainThreshold
			}
		}

		// 使用 Kadane 算法找到最大子数组
		maxSum, start, end := p.kadaneAlgorithm(adjustedScores)

		// 如果最大和小于停止阈值，停止
		if maxSum < p.strategy.StoppingThreshold {
			break
		}

		// 标记已使用
		for i := start; i <= end; i++ {
			used[i] = true
		}

		// 创建片段
		segmentMsgs := make([]Message, end-start+1)
		copy(segmentMsgs, msgs[start:end+1])

		segments = append(segments, Segment{
			StartIndex: start,
			EndIndex:   end,
			Score:      maxSum,
			Messages:   segmentMsgs,
		})
	}

	// 按原始顺序排序片段
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].StartIndex < segments[j].StartIndex
	})

	return segments
}

// kadaneAlgorithm 经典 Kadane 算法
func (p *AdvancedContextPruner) kadaneAlgorithm(scores []float64) (maxSum float64, start, end int) {
	if len(scores) == 0 {
		return 0, 0, 0
	}

	maxSum = scores[0]
	currentSum := scores[0]
	start = 0
	end = 0
	tempStart := 0

	for i := 1; i < len(scores); i++ {
		if currentSum < 0 {
			currentSum = scores[i]
			tempStart = i
		} else {
			currentSum += scores[i]
		}

		if currentSum > maxSum {
			maxSum = currentSum
			start = tempStart
			end = i
		}
	}

	return maxSum, start, end
}

// selectSegments 选择片段直到达到 token 限制
func (p *AdvancedContextPruner) selectSegments(segments []Segment, maxTokens int) []Message {
	if len(segments) == 0 {
		return nil
	}

	// 按得分排序（保留原始顺序信息）
	type scoredSegment struct {
		segment Segment
		index   int
	}

	scored := make([]scoredSegment, len(segments))
	for i, seg := range segments {
		scored[i] = scoredSegment{segment: seg, index: i}
	}

	// 按得分降序排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].segment.Score > scored[j].segment.Score
	})

	// 选择片段
	selected := []scoredSegment{}
	currentTokens := 0

	for _, ss := range scored {
		segmentTokens := p.estimateSegmentTokens(ss.segment.Messages)
		if currentTokens+segmentTokens <= maxTokens {
			selected = append(selected, ss)
			currentTokens += segmentTokens
		}
	}

	// 恢复原始顺序
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].index < selected[j].index
	})

	// 合并所有消息
	result := []Message{}
	for _, ss := range selected {
		result = append(result, ss.segment.Messages...)
	}

	return result
}

// estimateSegmentTokens 估算片段 token 数
func (p *AdvancedContextPruner) estimateSegmentTokens(msgs []Message) int {
	// 简化估算：每个字符约 0.25 token
	totalChars := 0
	for _, msg := range msgs {
		totalChars += len(msg.Content)
	}
	return totalChars / 4
}

// extractKeywords 提取关键词（辅助函数）
func extractKeywords(text string) []string {
	// 简化版：分词并过滤停用词
	// 生产环境应使用专业分词器
	words := []string{}
	// 这里省略具体实现，返回空切片
	return words
}

package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"go.uber.org/zap"
)

// 迭代深化Config配置了迭代深化研究推理模式.
// 在深层研究的循环探索方法的启发下,这种模式
// 进行宽度第一查询生成,然后进行深度第一回溯探索。
type IterativeDeepeningConfig struct {
	Breadth         int           // Number of parallel search queries per level
	MaxDepth        int           // Maximum recursion depth for exploration
	ResultsPerQuery int           // Maximum results to analyze per query
	MinConfidence   float64       // Minimum confidence to continue deepening (0-1)
	Timeout         time.Duration // Overall timeout for the entire research process
	ParallelSearch  bool          // Whether to execute searches in parallel
	SynthesisModel  string        // Model to use for final synthesis (empty = default)
}

// 默认的深度 Config 返回用于迭代深化研究的合理默认值 。
func DefaultIterativeDeepeningConfig() IterativeDeepeningConfig {
	return IterativeDeepeningConfig{
		Breadth:         3,
		MaxDepth:        3,
		ResultsPerQuery: 5,
		MinConfidence:   0.3,
		Timeout:         180 * time.Second,
		ParallelSearch:  true,
		SynthesisModel:  "",
	}
}

// 迭代深化实施迭代深化研究推理模式.
// 它通过生成搜索查询,分析结果,来循环地探索一个话题,
// 确定新的研究方向,深化探索直至充分
// 实现理解或达到深度限制。
//
// 这种模式对以下方面特别有效:
// - 需要多方面探索的不限成员名额的研究问题
// - 初步询问显示意外分专题的专题
// - 综合文献/信息调查
// - 通过逐步完善,加深理解
type IterativeDeepening struct {
	provider     llm.Provider
	toolExecutor tools.ToolExecutor
	config       IterativeDeepeningConfig
	logger       *zap.Logger
}

// NewIterative Deepenning 创造了一个新的"活泼"Deepenning Research causeer. 互联网档案馆的存檔,存档日期2013-12-21.
func NewIterativeDeepening(provider llm.Provider, executor tools.ToolExecutor, config IterativeDeepeningConfig, logger *zap.Logger) *IterativeDeepening {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &IterativeDeepening{
		provider:     provider,
		toolExecutor: executor,
		config:       config,
		logger:       logger,
	}
}

// 名称返回模式标识符 。
func (id *IterativeDeepening) Name() string { return "iterative_deepening" }

// 寻找是研究查询得出的单一发现。
type researchFinding struct {
	Query     string  `json:"query"`
	Finding   string  `json:"finding"`
	Source    string  `json:"source,omitempty"`
	Relevance float64 `json:"relevance"`
}

// 研究方向是探索的新方向。
type researchDirection struct {
	Query      string  `json:"query"`
	Rationale  string  `json:"rationale"`
	Priority   float64 `json:"priority"`
}

// Execute运行"惯性深化研究"推理模式.
func (id *IterativeDeepening) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, id.config.Timeout)
	defer cancel()

	result := &ReasoningResult{
		Pattern:  id.Name(),
		Task:     task,
		Metadata: make(map[string]any),
	}

	// 第一阶段:提出初步研究询问
	id.logger.Info("starting iterative deepening research",
		zap.String("task", truncate(task, 100)),
		zap.Int("breadth", id.config.Breadth),
		zap.Int("max_depth", id.config.MaxDepth))

	queries, tokens, err := id.generateQueries(ctx, task, nil, id.config.Breadth)
	if err != nil {
		return nil, fmt.Errorf("failed to generate initial queries: %w", err)
	}
	result.TotalTokens += tokens

	result.Steps = append(result.Steps, ReasoningStep{
		StepID:  fmt.Sprintf("init_%d", time.Now().UnixNano()),
		Type:    "thought",
		Content: fmt.Sprintf("Generated %d initial research queries for: %s", len(queries), truncate(task, 80)),
	})

	// 第2阶段:循环勘探
	allFindings := make([]researchFinding, 0)
	exploredQueries := make(map[string]bool)

	err = id.explore(ctx, task, queries, 0, &allFindings, exploredQueries, result)
	if err != nil {
		id.logger.Warn("exploration ended with error", zap.Error(err))
		// 不要完全失败 我们可能会有部分结果
	}

	// 第3阶段:将调查结果合成最终答案
	id.logger.Info("synthesizing findings",
		zap.Int("total_findings", len(allFindings)),
		zap.Int("explored_queries", len(exploredQueries)))
	synthesis, synthTokens, err := id.synthesize(ctx, task, allFindings)
	if err != nil {
		// 倒置: 连结结果
		var sb strings.Builder
		for _, f := range allFindings {
			sb.WriteString(fmt.Sprintf("- %s\n", f.Finding))
		}
		result.FinalAnswer = sb.String()
	} else {
		result.FinalAnswer = synthesis
		result.TotalTokens += synthTokens
	}

	result.Steps = append(result.Steps, ReasoningStep{
		StepID:  fmt.Sprintf("synthesis_%d", time.Now().UnixNano()),
		Type:    "evaluation",
		Content: fmt.Sprintf("Synthesized %d findings from %d queries into final answer", len(allFindings), len(exploredQueries)),
	})

	result.TotalLatency = time.Since(start)
	result.Confidence = id.calculateConfidence(allFindings)
	result.Metadata["total_findings"] = len(allFindings)
	result.Metadata["explored_queries"] = len(exploredQueries)
	result.Metadata["max_depth_used"] = id.config.MaxDepth

	id.logger.Info("iterative deepening completed",
		zap.Duration("latency", result.TotalLatency),
		zap.Int("tokens", result.TotalTokens),
		zap.Float64("confidence", result.Confidence))

	return result, nil
}

// 探索递归地探索研究方向,深度不断提高.
func (id *IterativeDeepening) explore(
	ctx context.Context,
	originalTask string,
	queries []string,
	depth int,
	findings *[]researchFinding,
	explored map[string]bool,
	result *ReasoningResult,
) error {
	if depth >= id.config.MaxDepth {
		id.logger.Debug("max depth reached", zap.Int("depth", depth))
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	id.logger.Debug("exploring depth level",
		zap.Int("depth", depth),
		zap.Int("queries", len(queries)))

	// 执行查询(平行或相继)
	levelFindings, tokens := id.executeQueries(ctx, queries, explored, result)
	result.TotalTokens += tokens

	// 附加调查结果
	*findings = append(*findings, levelFindings...)

	result.Steps = append(result.Steps, ReasoningStep{
		StepID:     fmt.Sprintf("depth_%d_%d", depth, time.Now().UnixNano()),
		Type:       "observation",
		Content:    fmt.Sprintf("Depth %d: found %d findings from %d queries", depth, len(levelFindings), len(queries)),
		TokensUsed: tokens,
	})

	// 检查我们是否有足够的信心 提早停止
	if id.calculateConfidence(levelFindings) >= 0.9 {
		id.logger.Info("high confidence reached, stopping early", zap.Int("depth", depth))
		return nil
	}

	// 根据研究结果制定新的研究方向
	directions, dirTokens, err := id.generateDirections(ctx, originalTask, levelFindings)
	if err != nil {
		id.logger.Warn("failed to generate new directions", zap.Error(err))
		return nil // Don't fail, just stop deepening
	}
	result.TotalTokens += dirTokens

	// 过滤已探索的方向
	newQueries := make([]string, 0)
	for _, dir := range directions {
		if !explored[dir.Query] && dir.Priority >= id.config.MinConfidence {
			newQueries = append(newQueries, dir.Query)
		}
	}

	if len(newQueries) == 0 {
		id.logger.Debug("no new directions to explore", zap.Int("depth", depth))
		return nil
	}

	// 限制宽度
	if len(newQueries) > id.config.Breadth {
		newQueries = newQueries[:id.config.Breadth]
	}

	result.Steps = append(result.Steps, ReasoningStep{
		StepID:  fmt.Sprintf("branch_%d_%d", depth, time.Now().UnixNano()),
		Type:    "thought",
		Content: fmt.Sprintf("Depth %d: identified %d new research directions, exploring %d", depth, len(directions), len(newQueries)),
	})

	// 更深入地递回
	return id.explore(ctx, originalTask, newQueries, depth+1, findings, explored, result)
}

// 执行查询运行搜索查询并提取发现 。
func (id *IterativeDeepening) executeQueries(
	ctx context.Context,
	queries []string,
	explored map[string]bool,
	result *ReasoningResult,
) ([]researchFinding, int) {
	var allFindings []researchFinding
	var totalTokens int
	var mu sync.Mutex

	// 过滤已探索的查询
	newQueries := make([]string, 0)
	for _, q := range queries {
		if !explored[q] {
			newQueries = append(newQueries, q)
			explored[q] = true
		}
	}

	if !id.config.ParallelSearch {
		// 顺序处决
		for _, query := range newQueries {
			findings, tokens, err := id.analyzeQuery(ctx, query)
			if err != nil {
				id.logger.Warn("query analysis failed", zap.String("query", truncate(query, 50)), zap.Error(err))
				continue
			}
			allFindings = append(allFindings, findings...)
			totalTokens += tokens
		}
		return allFindings, totalTokens
	}

	// 并行执行
	var wg sync.WaitGroup
	for _, query := range newQueries {
		wg.Add(1)
		go func(q string) {
			defer wg.Done()
			findings, tokens, err := id.analyzeQuery(ctx, q)
			if err != nil {
				id.logger.Warn("query analysis failed", zap.String("query", truncate(q, 50)), zap.Error(err))
				return
			}
			mu.Lock()
			allFindings = append(allFindings, findings...)
			totalTokens += tokens
			mu.Unlock()
		}(query)
	}
	wg.Wait()

	return allFindings, totalTokens
}

// 生成查询为特定任务和上下文生成搜索查询。
func (id *IterativeDeepening) generateQueries(ctx context.Context, task string, context []researchFinding, count int) ([]string, int, error) {
	prompt := fmt.Sprintf(`You are a research assistant. Generate %d specific, targeted search queries to investigate the following topic.

Topic: %s`, count, task)

	if len(context) > 0 {
		var contextStr strings.Builder
		for _, f := range context {
			contextStr.WriteString(fmt.Sprintf("- %s\n", f.Finding))
		}
		prompt += fmt.Sprintf(`

Previous findings:
%s

Generate queries that explore NEW aspects not covered by previous findings.`, contextStr.String())
	}

	prompt += `

Respond as a JSON array of strings, e.g.: ["query 1", "query 2", "query 3"]`

	resp, err := id.provider.Completion(ctx, &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.7,
		MaxTokens:   500,
	})
	if err != nil {
		return nil, 0, err
	}

	tokens := resp.Usage.TotalTokens
	genChoice, choiceErr := llm.FirstChoice(resp)
	if choiceErr != nil {
		return nil, tokens, fmt.Errorf("query generation returned no choices: %w", choiceErr)
	}
	content := genChoice.Message.Content

	var queries []string
	jsonStr := extractJSONObject(content)
	if err := json.Unmarshal([]byte(jsonStr), &queries); err != nil {
		// 倒置: 被新行分割
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "]") {
				line = strings.Trim(line, `",-`)
				if line != "" {
					queries = append(queries, line)
				}
			}
		}
	}

	if len(queries) > count {
		queries = queries[:count]
	}

	return queries, tokens, nil
}

// 分析 Query 分析单个查询并提取发现.
func (id *IterativeDeepening) analyzeQuery(ctx context.Context, query string) ([]researchFinding, int, error) {
	prompt := fmt.Sprintf(`You are a research analyst. Analyze the following research query and provide key findings.

Query: %s

Provide your analysis as a JSON array of findings:
[
  {"finding": "key insight 1", "relevance": 0.9, "source": "reasoning"},
  {"finding": "key insight 2", "relevance": 0.7, "source": "reasoning"}
]

Focus on factual, specific, and actionable insights. Rate relevance from 0.0 to 1.0.`, query)

	resp, err := id.provider.Completion(ctx, &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   800,
	})
	if err != nil {
		return nil, 0, err
	}

	tokens := resp.Usage.TotalTokens
	analyzeChoice, choiceErr := llm.FirstChoice(resp)
	if choiceErr != nil {
		return nil, tokens, fmt.Errorf("query analysis returned no choices: %w", choiceErr)
	}
	content := analyzeChoice.Message.Content

	var findings []researchFinding
	jsonStr := extractJSONObject(content)
	if err := json.Unmarshal([]byte(jsonStr), &findings); err != nil {
		// 倒置: 将整个响应视为单一发现
		findings = []researchFinding{{
			Query:     query,
			Finding:   content,
			Source:    "llm_analysis",
			Relevance: 0.5,
		}}
	}

	// 用源码查询标记结果
	for i := range findings {
		findings[i].Query = query
	}

	// 限制每次查询的结果
	if len(findings) > id.config.ResultsPerQuery {
		findings = findings[:id.config.ResultsPerQuery]
	}

	return findings, tokens, nil
}

// 生成指令根据研究结果确定新的研究方向。
func (id *IterativeDeepening) generateDirections(ctx context.Context, task string, findings []researchFinding) ([]researchDirection, int, error) {
	var findingsStr strings.Builder
	for _, f := range findings {
		findingsStr.WriteString(fmt.Sprintf("- [%.1f] %s\n", f.Relevance, f.Finding))
	}
	prompt := fmt.Sprintf(`You are a research strategist. Based on the original task and current findings, identify new research directions that would deepen our understanding.

Original task: %s

Current findings:
%s

Identify %d new research directions. For each, provide a specific search query and rationale.

Respond as JSON array:
[
  {"query": "specific search query", "rationale": "why this direction matters", "priority": 0.8},
  ...
]

Priority should be 0.0-1.0 based on how important this direction is.`, task, findingsStr.String(), id.config.Breadth)

	resp, err := id.provider.Completion(ctx, &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.6,
		MaxTokens:   600,
	})
	if err != nil {
		return nil, 0, err
	}

	tokens := resp.Usage.TotalTokens
	dirChoice, choiceErr := llm.FirstChoice(resp)
	if choiceErr != nil {
		return nil, tokens, fmt.Errorf("direction generation returned no choices: %w", choiceErr)
	}
	content := dirChoice.Message.Content

	var directions []researchDirection
	jsonStr := extractJSONObject(content)
	if err := json.Unmarshal([]byte(jsonStr), &directions); err != nil {
		return nil, tokens, fmt.Errorf("failed to parse directions: %w", err)
	}

	return directions, tokens, nil
}

// 将所有结论综合为一个全面的最终答案。
func (id *IterativeDeepening) synthesize(ctx context.Context, task string, findings []researchFinding) (string, int, error) {
	var findingsStr strings.Builder
	for i, f := range findings {
		findingsStr.WriteString(fmt.Sprintf("%d. [Relevance: %.1f] %s\n", i+1, f.Relevance, f.Finding))
	}
	prompt := fmt.Sprintf(`You are a research synthesizer. Based on extensive research findings, provide a comprehensive, well-structured answer to the original question.

Original question: %s

Research findings:
%s

Synthesize these findings into a clear, comprehensive answer. Include:
1. A direct answer to the question
2. Key supporting evidence
3. Important nuances or caveats
4. Areas where more research might be needed

Be thorough but concise.`, task, findingsStr.String())

	resp, err := id.provider.Completion(ctx, &llm.ChatRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   2000,
	})
	if err != nil {
		return "", 0, err
	}

	synthChoice, choiceErr := llm.FirstChoice(resp)
	if choiceErr != nil {
		return "", resp.Usage.TotalTokens, fmt.Errorf("synthesis returned no choices: %w", choiceErr)
	}
	return synthChoice.Message.Content, resp.Usage.TotalTokens, nil
}

// 根据调查结果计算总体信心。
func (id *IterativeDeepening) calculateConfidence(findings []researchFinding) float64 {
	if len(findings) == 0 {
		return 0.0
	}

	var totalRelevance float64
	for _, f := range findings {
		totalRelevance += f.Relevance
	}

	avgRelevance := totalRelevance / float64(len(findings))

	// 数量因素:调查结果多=信心高(回报减少)
	quantityFactor := 1.0 - 1.0/float64(1+len(findings))

	confidence := avgRelevance*0.6 + quantityFactor*0.4
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

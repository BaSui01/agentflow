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

// IterativeDeepeningConfig configures the Iterative Deepening Research reasoning pattern.
// Inspired by deep-research's recursive exploration approach, this pattern
// performs breadth-first query generation followed by depth-first recursive exploration.
type IterativeDeepeningConfig struct {
	Breadth         int           // Number of parallel search queries per level
	MaxDepth        int           // Maximum recursion depth for exploration
	ResultsPerQuery int           // Maximum results to analyze per query
	MinConfidence   float64       // Minimum confidence to continue deepening (0-1)
	Timeout         time.Duration // Overall timeout for the entire research process
	ParallelSearch  bool          // Whether to execute searches in parallel
	SynthesisModel  string        // Model to use for final synthesis (empty = default)
}

// DefaultIterativeDeepeningConfig returns sensible defaults for iterative deepening research.
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

// IterativeDeepening implements the Iterative Deepening Research reasoning pattern.
// It recursively explores a topic by generating search queries, analyzing results,
// identifying new research directions, and deepening exploration until sufficient
// understanding is achieved or depth limits are reached.
//
// This pattern is particularly effective for:
// - Open-ended research questions requiring multi-faceted exploration
// - Topics where initial queries reveal unexpected sub-topics
// - Comprehensive literature/information surveys
// - Building deep understanding through progressive refinement
type IterativeDeepening struct {
	provider     llm.Provider
	toolExecutor tools.ToolExecutor
	config       IterativeDeepeningConfig
	logger       *zap.Logger
}

// NewIterativeDeepening creates a new Iterative Deepening Research reasoner.
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

// Name returns the pattern identifier.
func (id *IterativeDeepening) Name() string { return "iterative_deepening" }

// researchFinding represents a single finding from a research query.
type researchFinding struct {
	Query     string  `json:"query"`
	Finding   string  `json:"finding"`
	Source    string  `json:"source,omitempty"`
	Relevance float64 `json:"relevance"`
}

// researchDirection represents a new direction to explore.
type researchDirection struct {
	Query      string  `json:"query"`
	Rationale  string  `json:"rationale"`
	Priority   float64 `json:"priority"`
}

// Execute runs the Iterative Deepening Research reasoning pattern.
func (id *IterativeDeepening) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, id.config.Timeout)
	defer cancel()

	result := &ReasoningResult{
		Pattern:  id.Name(),
		Task:     task,
		Metadata: make(map[string]any),
	}

	// Phase 1: Generate initial research queries
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

	// Phase 2: Recursive exploration
	allFindings := make([]researchFinding, 0)
	exploredQueries := make(map[string]bool)

	err = id.explore(ctx, task, queries, 0, &allFindings, exploredQueries, result)
	if err != nil {
		id.logger.Warn("exploration ended with error", zap.Error(err))
		// Don't fail entirely - we may have partial results
	}

	// Phase 3: Synthesize findings into final answer
	id.logger.Info("synthesizing findings",
		zap.Int("total_findings", len(allFindings)),
		zap.Int("explored_queries", len(exploredQueries)))
	synthesis, synthTokens, err := id.synthesize(ctx, task, allFindings)
	if err != nil {
		// Fallback: concatenate findings
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

// explore recursively explores research directions at increasing depth.
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

	// Execute queries (parallel or sequential)
	levelFindings, tokens := id.executeQueries(ctx, queries, explored, result)
	result.TotalTokens += tokens

	// Append findings
	*findings = append(*findings, levelFindings...)

	result.Steps = append(result.Steps, ReasoningStep{
		StepID:     fmt.Sprintf("depth_%d_%d", depth, time.Now().UnixNano()),
		Type:       "observation",
		Content:    fmt.Sprintf("Depth %d: found %d findings from %d queries", depth, len(levelFindings), len(queries)),
		TokensUsed: tokens,
	})

	// Check if we have enough confidence to stop early
	if id.calculateConfidence(levelFindings) >= 0.9 {
		id.logger.Info("high confidence reached, stopping early", zap.Int("depth", depth))
		return nil
	}

	// Generate new research directions based on findings
	directions, dirTokens, err := id.generateDirections(ctx, originalTask, levelFindings)
	if err != nil {
		id.logger.Warn("failed to generate new directions", zap.Error(err))
		return nil // Don't fail, just stop deepening
	}
	result.TotalTokens += dirTokens

	// Filter out already-explored directions
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

	// Limit to breadth
	if len(newQueries) > id.config.Breadth {
		newQueries = newQueries[:id.config.Breadth]
	}

	result.Steps = append(result.Steps, ReasoningStep{
		StepID:  fmt.Sprintf("branch_%d_%d", depth, time.Now().UnixNano()),
		Type:    "thought",
		Content: fmt.Sprintf("Depth %d: identified %d new research directions, exploring %d", depth, len(directions), len(newQueries)),
	})

	// Recurse deeper
	return id.explore(ctx, originalTask, newQueries, depth+1, findings, explored, result)
}

// executeQueries runs search queries and extracts findings.
func (id *IterativeDeepening) executeQueries(
	ctx context.Context,
	queries []string,
	explored map[string]bool,
	result *ReasoningResult,
) ([]researchFinding, int) {
	var allFindings []researchFinding
	var totalTokens int
	var mu sync.Mutex

	// Filter already-explored queries
	newQueries := make([]string, 0)
	for _, q := range queries {
		if !explored[q] {
			newQueries = append(newQueries, q)
			explored[q] = true
		}
	}

	if !id.config.ParallelSearch {
		// Sequential execution
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

	// Parallel execution
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

// generateQueries generates search queries for a given task and context.
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
	content := resp.Choices[0].Message.Content

	var queries []string
	jsonStr := extractJSONObject(content)
	if err := json.Unmarshal([]byte(jsonStr), &queries); err != nil {
		// Fallback: split by newlines
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

// analyzeQuery analyzes a single query and extracts findings.
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
	content := resp.Choices[0].Message.Content

	var findings []researchFinding
	jsonStr := extractJSONObject(content)
	if err := json.Unmarshal([]byte(jsonStr), &findings); err != nil {
		// Fallback: treat entire response as single finding
		findings = []researchFinding{{
			Query:     query,
			Finding:   content,
			Source:    "llm_analysis",
			Relevance: 0.5,
		}}
	}

	// Tag findings with their source query
	for i := range findings {
		findings[i].Query = query
	}

	// Limit results per query
	if len(findings) > id.config.ResultsPerQuery {
		findings = findings[:id.config.ResultsPerQuery]
	}

	return findings, tokens, nil
}

// generateDirections identifies new research directions based on findings.
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
	content := resp.Choices[0].Message.Content

	var directions []researchDirection
	jsonStr := extractJSONObject(content)
	if err := json.Unmarshal([]byte(jsonStr), &directions); err != nil {
		return nil, tokens, fmt.Errorf("failed to parse directions: %w", err)
	}

	return directions, tokens, nil
}

// synthesize combines all findings into a comprehensive final answer.
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

	return resp.Choices[0].Message.Content, resp.Usage.TotalTokens, nil
}

// calculateConfidence computes overall confidence based on findings.
func (id *IterativeDeepening) calculateConfidence(findings []researchFinding) float64 {
	if len(findings) == 0 {
		return 0.0
	}

	var totalRelevance float64
	for _, f := range findings {
		totalRelevance += f.Relevance
	}

	avgRelevance := totalRelevance / float64(len(findings))

	// Factor in quantity: more findings = higher confidence (with diminishing returns)
	quantityFactor := 1.0 - 1.0/float64(1+len(findings))

	confidence := avgRelevance*0.6 + quantityFactor*0.4
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

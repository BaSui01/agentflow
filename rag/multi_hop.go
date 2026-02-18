// Package rag provides multi-hop reasoning capabilities for complex queries.
// This module implements iterative retrieval with context passing and reasoning chain tracking.
package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ====== Multi-Hop Reasoning Types ======

// HopType represents the type of reasoning hop
type HopType string

const (
	HopTypeInitial      HopType = "initial"       // Initial query retrieval
	HopTypeFollowUp     HopType = "follow_up"     // Follow-up based on previous results
	HopTypeDecomposed   HopType = "decomposed"    // Sub-query from decomposition
	HopTypeRefinement   HopType = "refinement"    // Query refinement based on context
	HopTypeVerification HopType = "verification"  // Verify or cross-check information
	HopTypeBridging     HopType = "bridging"      // Bridge between concepts
)

// ReasoningStatus represents the status of the reasoning process
type ReasoningStatus string

const (
	StatusInProgress ReasoningStatus = "in_progress"
	StatusCompleted  ReasoningStatus = "completed"
	StatusFailed     ReasoningStatus = "failed"
	StatusTimeout    ReasoningStatus = "timeout"
)

// ReasoningHop represents a single hop in the reasoning chain
type ReasoningHop struct {
	ID            string            `json:"id"`
	HopNumber     int               `json:"hop_number"`
	Type          HopType           `json:"type"`
	Query         string            `json:"query"`
	TransformedQuery string         `json:"transformed_query,omitempty"`
	Results       []RetrievalResult `json:"results"`
	Context       string            `json:"context,omitempty"`       // Accumulated context
	Reasoning     string            `json:"reasoning,omitempty"`     // LLM reasoning for this hop
	Confidence    float64           `json:"confidence"`
	Duration      time.Duration     `json:"duration"`
	Metadata      map[string]any    `json:"metadata,omitempty"`
	Timestamp     time.Time         `json:"timestamp"`

	// 去重统计（新增）
	DedupStats *DedupStats `json:"dedup_stats,omitempty"`
}

// DedupStats 去重统计
type DedupStats struct {
	TotalRetrieved    int `json:"total_retrieved"`     // 原始检索结果数
	DedupByID         int `json:"dedup_by_id"`         // 按 ID 去重数量
	DedupBySimilarity int `json:"dedup_by_similarity"` // 按内容相似度去重数量
	FinalCount        int `json:"final_count"`         // 去重后最终数量
}

// ReasoningChain represents the complete reasoning chain
type ReasoningChain struct {
	ID              string          `json:"id"`
	OriginalQuery   string          `json:"original_query"`
	Hops            []ReasoningHop  `json:"hops"`
	FinalAnswer     string          `json:"final_answer,omitempty"`
	FinalContext    string          `json:"final_context"`
	Status          ReasoningStatus `json:"status"`
	TotalDuration   time.Duration   `json:"total_duration"`
	TotalRetrieval  int             `json:"total_retrieval"`  // Total documents retrieved
	UniqueDocuments int             `json:"unique_documents"` // Unique documents
	Metadata        map[string]any  `json:"metadata,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	CompletedAt     time.Time       `json:"completed_at,omitempty"`

	// 全局去重统计（新增）
	TotalDedupByID         int `json:"total_dedup_by_id"`
	TotalDedupBySimilarity int `json:"total_dedup_by_similarity"`
}

// MultiHopConfig configures the multi-hop reasoning system
type MultiHopConfig struct {
	// Hop limits
	MaxHops           int           `json:"max_hops"`            // Maximum number of hops (2-5)
	MinHops           int           `json:"min_hops"`            // Minimum hops before stopping
	HopTimeout        time.Duration `json:"hop_timeout"`         // Timeout per hop
	TotalTimeout      time.Duration `json:"total_timeout"`       // Total reasoning timeout

	// Retrieval settings
	ResultsPerHop     int     `json:"results_per_hop"`     // Documents per hop
	MinConfidence     float64 `json:"min_confidence"`      // Minimum confidence to continue
	ContextWindowSize int     `json:"context_window_size"` // Max context tokens

	// Reasoning settings
	EnableLLMReasoning    bool    `json:"enable_llm_reasoning"`    // Use LLM for reasoning
	EnableQueryRefinement bool    `json:"enable_query_refinement"` // Refine queries between hops
	EnableVerification    bool    `json:"enable_verification"`     // Verify answers
	ConfidenceThreshold   float64 `json:"confidence_threshold"`    // Stop if confidence exceeds

	// Deduplication
	DeduplicateResults bool    `json:"deduplicate_results"` // Remove duplicate documents
	SimilarityThreshold float64 `json:"similarity_threshold"` // Threshold for deduplication

	// Caching
	EnableCache bool          `json:"enable_cache"`
	CacheTTL    time.Duration `json:"cache_ttl"`
}

// DefaultMultiHopConfig returns default configuration
func DefaultMultiHopConfig() MultiHopConfig {
	return MultiHopConfig{
		MaxHops:               4,
		MinHops:               1,
		HopTimeout:            30 * time.Second,
		TotalTimeout:          2 * time.Minute,
		ResultsPerHop:         5,
		MinConfidence:         0.3,
		ContextWindowSize:     4000,
		EnableLLMReasoning:    true,
		EnableQueryRefinement: true,
		EnableVerification:    false,
		ConfidenceThreshold:   0.9,
		DeduplicateResults:    true,
		SimilarityThreshold:   0.85,
		EnableCache:           true,
		CacheTTL:              15 * time.Minute,
	}
}

// ====== Multi-Hop Reasoner ======

// MultiHopReasoner performs multi-hop reasoning over documents
type MultiHopReasoner struct {
	config           MultiHopConfig
	retriever        *HybridRetriever
	queryTransformer *QueryTransformer
	llmProvider      QueryLLMProvider
	embeddingFunc    func(context.Context, string) ([]float64, error)
	cache            *reasoningCache
	logger           *zap.Logger
}

// reasoningCache caches reasoning chains
type reasoningCache struct {
	entries map[string]*ReasoningChain
	mu      sync.RWMutex
	ttl     time.Duration
}

func newReasoningCache(ttl time.Duration) *reasoningCache {
	return &reasoningCache{
		entries: make(map[string]*ReasoningChain),
		ttl:     ttl,
	}
}

func (c *reasoningCache) get(key string) (*ReasoningChain, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	chain, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	// Check if expired
	if time.Since(chain.CreatedAt) > c.ttl {
		return nil, false
	}

	return chain, true
}

func (c *reasoningCache) set(key string, chain *ReasoningChain) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = chain
}

// NewMultiHopReasoner creates a new multi-hop reasoner
func NewMultiHopReasoner(
	config MultiHopConfig,
	retriever *HybridRetriever,
	queryTransformer *QueryTransformer,
	llmProvider QueryLLMProvider,
	embeddingFunc func(context.Context, string) ([]float64, error),
	logger *zap.Logger,
) *MultiHopReasoner {
	if logger == nil {
		logger = zap.NewNop()
	}

	var cache *reasoningCache
	if config.EnableCache {
		cache = newReasoningCache(config.CacheTTL)
	}

	return &MultiHopReasoner{
		config:           config,
		retriever:        retriever,
		queryTransformer: queryTransformer,
		llmProvider:      llmProvider,
		embeddingFunc:    embeddingFunc,
		cache:            cache,
		logger:           logger.With(zap.String("component", "multi_hop_reasoner")),
	}
}

// Reason performs multi-hop reasoning for a query
func (r *MultiHopReasoner) Reason(ctx context.Context, query string) (*ReasoningChain, error) {
	// Check cache
	if r.cache != nil {
		if cached, ok := r.cache.get(query); ok {
			r.logger.Debug("cache hit", zap.String("query", query))
			return cached, nil
		}
	}

	// Create reasoning chain
	chain := &ReasoningChain{
		ID:            generateChainID(),
		OriginalQuery: query,
		Hops:          make([]ReasoningHop, 0),
		Status:        StatusInProgress,
		Metadata:      make(map[string]any),
		CreatedAt:     time.Now(),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, r.config.TotalTimeout)
	defer cancel()

	startTime := time.Now()
	seenDocIDs := make(map[string]bool)
	seenQueries := make(map[string]bool) // Track executed queries to prevent cycles
	accumulatedContext := ""

	// Initial query transformation
	var currentQuery string
	var subQueries []string

	if r.queryTransformer != nil {
		transformed, err := r.queryTransformer.Transform(ctx, query)
		if err != nil {
			r.logger.Warn("query transformation failed", zap.Error(err))
			currentQuery = query
		} else {
			currentQuery = transformed.Transformed
			subQueries = transformed.SubQueries
			chain.Metadata["intent"] = string(transformed.Intent)
			chain.Metadata["keywords"] = transformed.Keywords
		}
	} else {
		currentQuery = query
	}

	// Execute reasoning hops
	for hopNum := 0; hopNum < r.config.MaxHops; hopNum++ {
		select {
		case <-ctx.Done():
			chain.Status = StatusTimeout
			chain.TotalDuration = time.Since(startTime)
			return chain, ctx.Err()
		default:
		}

		// Determine hop type and query
		hopType := HopTypeInitial
		hopQuery := currentQuery

		if hopNum > 0 {
			if len(subQueries) > hopNum-1 {
				// Use decomposed sub-query
				hopType = HopTypeDecomposed
				hopQuery = subQueries[hopNum-1]
			} else if r.config.EnableQueryRefinement {
				// Generate refined query based on context
				hopType = HopTypeFollowUp
				refinedQuery, err := r.refineQuery(ctx, query, accumulatedContext, hopNum)
				if err != nil {
					r.logger.Warn("query refinement failed", zap.Error(err))
				} else {
					hopQuery = refinedQuery
				}
			}
		}

		// Check for duplicate query (cycle detection)
		normalizedQuery := normalizeQueryForDedup(hopQuery)
		if seenQueries[normalizedQuery] {
			r.logger.Debug("skipping duplicate query",
				zap.String("query", hopQuery),
				zap.Int("hop", hopNum))
			continue
		}
		seenQueries[normalizedQuery] = true

		// Execute hop
		hop, err := r.executeHop(ctx, hopNum, hopType, hopQuery, accumulatedContext, seenDocIDs)
		if err != nil {
			r.logger.Warn("hop execution failed",
				zap.Int("hop", hopNum),
				zap.Error(err))
			continue
		}

		chain.Hops = append(chain.Hops, *hop)

		// Update accumulated context
		accumulatedContext = r.updateContext(accumulatedContext, hop)

		// Track unique documents
		for _, result := range hop.Results {
			if !seenDocIDs[result.Document.ID] {
				seenDocIDs[result.Document.ID] = true
				chain.UniqueDocuments++
			}
			chain.TotalRetrieval++
		}

		// 汇总去重统计
		if hop.DedupStats != nil {
			chain.TotalDedupByID += hop.DedupStats.DedupByID
			chain.TotalDedupBySimilarity += hop.DedupStats.DedupBySimilarity
		}

		// Check stopping conditions
		if r.shouldStop(ctx, chain, hop, hopNum) {
			break
		}
	}

	// Generate final answer if LLM is available
	if r.config.EnableLLMReasoning && r.llmProvider != nil {
		finalAnswer, err := r.generateFinalAnswer(ctx, query, chain)
		if err != nil {
			r.logger.Warn("final answer generation failed", zap.Error(err))
		} else {
			chain.FinalAnswer = finalAnswer
		}
	}

	// Finalize chain
	chain.FinalContext = accumulatedContext
	chain.Status = StatusCompleted
	chain.TotalDuration = time.Since(startTime)
	chain.CompletedAt = time.Now()

	// Cache result
	if r.cache != nil {
		r.cache.set(query, chain)
	}

	r.logger.Info("reasoning completed",
		zap.String("query", query),
		zap.Int("hops", len(chain.Hops)),
		zap.Int("unique_docs", chain.UniqueDocuments),
		zap.Int("total_retrieved", chain.TotalRetrieval),
		zap.Int("dedup_by_id", chain.TotalDedupByID),
		zap.Int("dedup_by_similarity", chain.TotalDedupBySimilarity),
		zap.Duration("duration", chain.TotalDuration))

	return chain, nil
}

// executeHop executes a single reasoning hop
func (r *MultiHopReasoner) executeHop(
	ctx context.Context,
	hopNum int,
	hopType HopType,
	query string,
	previousContext string,
	seenDocIDs map[string]bool,
) (*ReasoningHop, error) {
	hopCtx, cancel := context.WithTimeout(ctx, r.config.HopTimeout)
	defer cancel()

	startTime := time.Now()

	hop := &ReasoningHop{
		ID:        fmt.Sprintf("hop_%d_%d", hopNum, time.Now().UnixNano()),
		HopNumber: hopNum,
		Type:      hopType,
		Query:     query,
		Results:   make([]RetrievalResult, 0),
		Metadata:  make(map[string]any),
		Timestamp: time.Now(),
	}

	// Transform query if transformer is available
	if r.queryTransformer != nil {
		transformed, err := r.queryTransformer.Transform(hopCtx, query)
		if err == nil {
			hop.TransformedQuery = transformed.Transformed
		}
	}

	// Generate query embedding
	var queryEmbedding []float64
	if r.embeddingFunc != nil {
		embedding, err := r.embeddingFunc(hopCtx, query)
		if err != nil {
			r.logger.Warn("embedding generation failed", zap.Error(err))
		} else {
			queryEmbedding = embedding
		}
	}

	// Retrieve documents
	results, err := r.retriever.Retrieve(hopCtx, query, queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	// Filter and deduplicate results
	stats := &DedupStats{
		TotalRetrieved: len(results),
	}

	// Phase 1: 基于文档 ID 去重 + 最低分数过滤
	idFilteredResults := make([]RetrievalResult, 0, len(results))
	hopSeenIDs := make(map[string]bool) // 同一 hop 内的 ID 去重
	for _, result := range results {
		// 跨 hop ID 去重
		if r.config.DeduplicateResults && seenDocIDs[result.Document.ID] {
			stats.DedupByID++
			continue
		}
		// 同一 hop 内 ID 去重
		if hopSeenIDs[result.Document.ID] {
			stats.DedupByID++
			continue
		}
		// 最低分数过滤
		if result.FinalScore < r.config.MinConfidence {
			continue
		}
		hopSeenIDs[result.Document.ID] = true
		idFilteredResults = append(idFilteredResults, result)
	}

	// Phase 2: 基于内容相似度去重
	filteredResults := r.deduplicateBySimilarity(hopCtx, idFilteredResults, stats)

	// Phase 3: 去重后重新排序（按 FinalScore 降序）
	sort.Slice(filteredResults, func(i, j int) bool {
		return filteredResults[i].FinalScore > filteredResults[j].FinalScore
	})

	// Phase 4: 截断到 ResultsPerHop
	if len(filteredResults) > r.config.ResultsPerHop {
		filteredResults = filteredResults[:r.config.ResultsPerHop]
	}

	stats.FinalCount = len(filteredResults)
	hop.DedupStats = stats

	hop.Results = filteredResults

	// Calculate hop confidence
	if len(filteredResults) > 0 {
		totalScore := 0.0
		for _, result := range filteredResults {
			totalScore += result.FinalScore
		}
		hop.Confidence = totalScore / float64(len(filteredResults))
	}

	// Generate reasoning for this hop if LLM is available
	if r.config.EnableLLMReasoning && r.llmProvider != nil {
		reasoning, err := r.generateHopReasoning(hopCtx, query, filteredResults, previousContext)
		if err != nil {
			r.logger.Warn("hop reasoning failed", zap.Error(err))
		} else {
			hop.Reasoning = reasoning
		}
	}

	hop.Duration = time.Since(startTime)

	r.logger.Debug("hop executed",
		zap.Int("hop_number", hopNum),
		zap.String("type", string(hopType)),
		zap.Int("results", len(filteredResults)),
		zap.Float64("confidence", hop.Confidence))

	return hop, nil
}

// deduplicateBySimilarity 基于内容相似度去重
// 使用文档 embedding 计算余弦相似度，超过阈值的视为重复
func (r *MultiHopReasoner) deduplicateBySimilarity(
	ctx context.Context,
	results []RetrievalResult,
	stats *DedupStats,
) []RetrievalResult {
	if !r.config.DeduplicateResults || len(results) <= 1 {
		return results
	}

	threshold := r.config.SimilarityThreshold
	if threshold <= 0 {
		threshold = 0.85
	}

	deduplicated := make([]RetrievalResult, 0, len(results))

	for _, candidate := range results {
		isDuplicate := false

		for _, existing := range deduplicated {
			similarity := r.computeContentSimilarity(ctx, candidate.Document, existing.Document)
			if similarity >= threshold {
				isDuplicate = true
				stats.DedupBySimilarity++

				// 如果候选文档分数更高，替换已有文档
				if candidate.FinalScore > existing.FinalScore {
					for i, d := range deduplicated {
						if d.Document.ID == existing.Document.ID {
							deduplicated[i] = candidate
							break
						}
					}
				}
				break
			}
		}

		if !isDuplicate {
			deduplicated = append(deduplicated, candidate)
		}
	}

	return deduplicated
}

// computeContentSimilarity 计算两个文档的内容相似度
// 优先使用 embedding 余弦相似度，fallback 到 Jaccard 相似度
func (r *MultiHopReasoner) computeContentSimilarity(
	ctx context.Context,
	doc1, doc2 Document,
) float64 {
	// 策略 1：如果两个文档都有 embedding，使用余弦相似度
	if len(doc1.Embedding) > 0 && len(doc2.Embedding) > 0 && len(doc1.Embedding) == len(doc2.Embedding) {
		return cosineSimilarity(doc1.Embedding, doc2.Embedding)
	}

	// 策略 2：如果有 embeddingFunc，动态生成 embedding
	if r.embeddingFunc != nil {
		emb1, err1 := r.embeddingFunc(ctx, doc1.Content)
		emb2, err2 := r.embeddingFunc(ctx, doc2.Content)
		if err1 == nil && err2 == nil && len(emb1) == len(emb2) {
			return cosineSimilarity(emb1, emb2)
		}
	}

	// 策略 3：Fallback 到 Jaccard 相似度（基于词集合）
	return jaccardSimilarity(doc1.Content, doc2.Content)
}

// jaccardSimilarity 计算 Jaccard 相似度（基于词集合）
func jaccardSimilarity(text1, text2 string) float64 {
	words1 := tokenizeToSet(text1)
	words2 := tokenizeToSet(text2)

	if len(words1) == 0 && len(words2) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range words1 {
		if words2[w] {
			intersection++
		}
	}

	union := len(words1) + len(words2) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// tokenizeToSet 将文本分词为集合
func tokenizeToSet(text string) map[string]bool {
	words := strings.Fields(strings.ToLower(text))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}

// refineQuery generates a refined query based on accumulated context
func (r *MultiHopReasoner) refineQuery(
	ctx context.Context,
	originalQuery string,
	context string,
	hopNum int,
) (string, error) {
	if r.llmProvider == nil {
		return originalQuery, nil
	}

	prompt := fmt.Sprintf(`Based on the original query and the information gathered so far, generate a follow-up query to find additional relevant information.

Original query: %s

Information gathered so far:
%s

This is hop %d of the reasoning process. Generate a focused follow-up query that:
1. Addresses gaps in the current information
2. Explores related aspects not yet covered
3. Seeks clarification or deeper details

Follow-up query:`, originalQuery, truncateContext(context, 2000), hopNum+1)

	response, err := r.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return originalQuery, err
	}

	return strings.TrimSpace(response), nil
}

// generateHopReasoning generates reasoning for a single hop
func (r *MultiHopReasoner) generateHopReasoning(
	ctx context.Context,
	query string,
	results []RetrievalResult,
	previousContext string,
) (string, error) {
	if len(results) == 0 {
		return "No relevant documents found for this query.", nil
	}

	// Build document summaries
	var docSummaries strings.Builder
	for i, result := range results {
		docSummaries.WriteString(fmt.Sprintf("\nDocument %d (score: %.2f):\n%s\n",
			i+1, result.FinalScore, truncateContext(result.Document.Content, 500)))
	}

	prompt := fmt.Sprintf(`Analyze the retrieved documents for the query and provide a brief reasoning summary.

Query: %s

Previous context:
%s

Retrieved documents:
%s

Provide a concise analysis (2-3 sentences) of:
1. What relevant information was found
2. How it relates to the query
3. What gaps remain

Analysis:`, query, truncateContext(previousContext, 1000), docSummaries.String())

	response, err := r.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

// generateFinalAnswer generates the final answer from the reasoning chain
func (r *MultiHopReasoner) generateFinalAnswer(
	ctx context.Context,
	query string,
	chain *ReasoningChain,
) (string, error) {
	// Build reasoning summary
	var reasoningSummary strings.Builder
	for _, hop := range chain.Hops {
		reasoningSummary.WriteString(fmt.Sprintf("\nHop %d (%s):\n", hop.HopNumber+1, hop.Type))
		reasoningSummary.WriteString(fmt.Sprintf("Query: %s\n", hop.Query))
		if hop.Reasoning != "" {
			reasoningSummary.WriteString(fmt.Sprintf("Findings: %s\n", hop.Reasoning))
		}
	}

	prompt := fmt.Sprintf(`Based on the multi-hop reasoning process, provide a comprehensive answer to the original query.

Original query: %s

Reasoning chain:
%s

Final context:
%s

Provide a clear, well-structured answer that:
1. Directly addresses the original query
2. Synthesizes information from all reasoning hops
3. Acknowledges any limitations or uncertainties

Answer:`, query, reasoningSummary.String(), truncateContext(chain.FinalContext, 2000))

	response, err := r.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(response), nil
}

// updateContext updates the accumulated context with new hop results
func (r *MultiHopReasoner) updateContext(currentContext string, hop *ReasoningHop) string {
	var newContext strings.Builder
	newContext.WriteString(currentContext)

	// Add hop results to context
	for _, result := range hop.Results {
		newContext.WriteString("\n---\n")
		newContext.WriteString(result.Document.Content)
	}

	// Add reasoning if available
	if hop.Reasoning != "" {
		newContext.WriteString("\n[Reasoning]: ")
		newContext.WriteString(hop.Reasoning)
	}

	// Truncate if too long
	contextStr := newContext.String()
	if len(contextStr) > r.config.ContextWindowSize*4 { // Approximate token to char ratio
		contextStr = contextStr[len(contextStr)-r.config.ContextWindowSize*4:]
	}

	return contextStr
}

// shouldStop determines if reasoning should stop
func (r *MultiHopReasoner) shouldStop(
	ctx context.Context,
	chain *ReasoningChain,
	lastHop *ReasoningHop,
	hopNum int,
) bool {
	// Minimum hops not reached
	if hopNum < r.config.MinHops-1 {
		return false
	}

	// High confidence reached
	if lastHop.Confidence >= r.config.ConfidenceThreshold {
		r.logger.Debug("stopping: confidence threshold reached",
			zap.Float64("confidence", lastHop.Confidence))
		return true
	}

	// No new results
	if len(lastHop.Results) == 0 {
		r.logger.Debug("stopping: no new results")
		return true
	}

	// Check if we have enough information (using LLM)
	if r.config.EnableLLMReasoning && r.llmProvider != nil && hopNum >= r.config.MinHops-1 {
		sufficient, err := r.checkSufficiency(ctx, chain)
		if err == nil && sufficient {
			r.logger.Debug("stopping: sufficient information gathered")
			return true
		}
	}

	return false
}

// checkSufficiency checks if gathered information is sufficient
func (r *MultiHopReasoner) checkSufficiency(ctx context.Context, chain *ReasoningChain) (bool, error) {
	prompt := fmt.Sprintf(`Given the original query and the information gathered through multi-hop reasoning, determine if we have sufficient information to answer the query.

Original query: %s

Information gathered:
%s

Respond with only "YES" if sufficient information is available, or "NO" if more retrieval is needed.`,
		chain.OriginalQuery, truncateContext(chain.FinalContext, 2000))

	response, err := r.llmProvider.Complete(ctx, prompt)
	if err != nil {
		return false, err
	}

	return strings.Contains(strings.ToUpper(response), "YES"), nil
}

// ====== Reasoning Chain Methods ======

// GetHop returns a specific hop by number
func (c *ReasoningChain) GetHop(hopNum int) *ReasoningHop {
	if hopNum < 0 || hopNum >= len(c.Hops) {
		return nil
	}
	return &c.Hops[hopNum]
}

// GetAllDocuments returns all unique documents from the chain
func (c *ReasoningChain) GetAllDocuments() []Document {
	seen := make(map[string]bool)
	docs := make([]Document, 0)

	for _, hop := range c.Hops {
		for _, result := range hop.Results {
			if !seen[result.Document.ID] {
				seen[result.Document.ID] = true
				docs = append(docs, result.Document)
			}
		}
	}

	return docs
}

// GetTopDocuments returns top-k documents by score across all hops
func (c *ReasoningChain) GetTopDocuments(k int) []RetrievalResult {
	// Collect all results
	allResults := make([]RetrievalResult, 0)
	seen := make(map[string]bool)

	for _, hop := range c.Hops {
		for _, result := range hop.Results {
			if !seen[result.Document.ID] {
				seen[result.Document.ID] = true
				allResults = append(allResults, result)
			}
		}
	}

	// Sort by score (optimized: O(n log n) instead of O(n²))
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].FinalScore > allResults[j].FinalScore
	})

	if k > len(allResults) {
		k = len(allResults)
	}

	return allResults[:k]
}

// ToJSON serializes the reasoning chain to JSON
func (c *ReasoningChain) ToJSON() ([]byte, error) {
	return json.Marshal(c)
}

// FromJSON deserializes a reasoning chain from JSON
func (c *ReasoningChain) FromJSON(data []byte) error {
	return json.Unmarshal(data, c)
}

// ====== Visualization ======

// ChainVisualization represents a visualization of the reasoning chain
type ChainVisualization struct {
	Nodes []VisualizationNode `json:"nodes"`
	Edges []VisualizationEdge `json:"edges"`
}

// VisualizationNode represents a node in the visualization
type VisualizationNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // "query", "hop", "document", "answer"
	Label    string         `json:"label"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// VisualizationEdge represents an edge in the visualization
type VisualizationEdge struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Label  string  `json:"label,omitempty"`
	Weight float64 `json:"weight,omitempty"`
}

// Visualize creates a visualization of the reasoning chain
func (c *ReasoningChain) Visualize() *ChainVisualization {
	viz := &ChainVisualization{
		Nodes: make([]VisualizationNode, 0),
		Edges: make([]VisualizationEdge, 0),
	}

	// Add query node
	queryNodeID := "query_0"
	viz.Nodes = append(viz.Nodes, VisualizationNode{
		ID:    queryNodeID,
		Type:  "query",
		Label: truncateContext(c.OriginalQuery, 50),
		Metadata: map[string]any{
			"full_query": c.OriginalQuery,
		},
	})

	prevNodeID := queryNodeID

	// Add hop nodes
	for _, hop := range c.Hops {
		hopNodeID := fmt.Sprintf("hop_%d", hop.HopNumber)
		viz.Nodes = append(viz.Nodes, VisualizationNode{
			ID:    hopNodeID,
			Type:  "hop",
			Label: fmt.Sprintf("Hop %d: %s", hop.HopNumber+1, hop.Type),
			Metadata: map[string]any{
				"query":      hop.Query,
				"confidence": hop.Confidence,
				"duration":   hop.Duration.String(),
			},
		})

		// Edge from previous node to hop
		viz.Edges = append(viz.Edges, VisualizationEdge{
			Source: prevNodeID,
			Target: hopNodeID,
			Label:  string(hop.Type),
		})

		// Add document nodes for this hop
		for i, result := range hop.Results {
			docNodeID := fmt.Sprintf("doc_%d_%d", hop.HopNumber, i)
			viz.Nodes = append(viz.Nodes, VisualizationNode{
				ID:    docNodeID,
				Type:  "document",
				Label: truncateContext(result.Document.Content, 30),
				Metadata: map[string]any{
					"doc_id": result.Document.ID,
					"score":  result.FinalScore,
				},
			})

			// Edge from hop to document
			viz.Edges = append(viz.Edges, VisualizationEdge{
				Source: hopNodeID,
				Target: docNodeID,
				Weight: result.FinalScore,
			})
		}

		prevNodeID = hopNodeID
	}

	// Add answer node if available
	if c.FinalAnswer != "" {
		answerNodeID := "answer_0"
		viz.Nodes = append(viz.Nodes, VisualizationNode{
			ID:    answerNodeID,
			Type:  "answer",
			Label: truncateContext(c.FinalAnswer, 50),
			Metadata: map[string]any{
				"full_answer": c.FinalAnswer,
			},
		})

		viz.Edges = append(viz.Edges, VisualizationEdge{
			Source: prevNodeID,
			Target: answerNodeID,
			Label:  "synthesize",
		})
	}

	return viz
}

// ====== Batch Processing ======

// ReasonBatch performs multi-hop reasoning for multiple queries
func (r *MultiHopReasoner) ReasonBatch(ctx context.Context, queries []string) ([]*ReasoningChain, error) {
	results := make([]*ReasoningChain, len(queries))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	// Limit concurrency
	semaphore := make(chan struct{}, 3)

	for i, query := range queries {
		wg.Add(1)
		go func(idx int, q string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			chain, err := r.Reason(ctx, q)
			mu.Lock()
			defer mu.Unlock()

			if err != nil && firstErr == nil {
				firstErr = err
			}
			results[idx] = chain
		}(i, query)
	}

	wg.Wait()
	return results, firstErr
}

// ====== Helper Functions ======

// generateChainID generates a unique chain ID
func generateChainID() string {
	return fmt.Sprintf("chain_%d", time.Now().UnixNano())
}

// truncateContext truncates context to a maximum length
func truncateContext(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// normalizeQueryForDedup normalizes a query for deduplication
// It converts to lowercase, trims whitespace, and normalizes spaces
func normalizeQueryForDedup(query string) string {
	// Convert to lowercase and trim
	query = strings.ToLower(strings.TrimSpace(query))
	// Normalize multiple spaces to single space
	return strings.Join(strings.Fields(query), " ")
}

package observability

import (
	"sync"
)

// CostCalculator 成本计算器
type CostCalculator struct {
	mu     sync.RWMutex
	prices map[string]*ModelPrice // key: provider:model
}

// ModelPrice 模型价格
type ModelPrice struct {
	Provider    string
	Model       string
	PriceInput  float64 // USD per 1K tokens
	PriceOutput float64 // USD per 1K tokens
}

// NewCostCalculator 创建成本计算器
func NewCostCalculator() *CostCalculator {
	c := &CostCalculator{
		prices: make(map[string]*ModelPrice),
	}
	c.loadDefaultPrices()
	return c
}

// loadDefaultPrices 加载默认价格（可从配置/数据库覆盖）
func (c *CostCalculator) loadDefaultPrices() {
	defaults := []ModelPrice{
		// OpenAI
		{Provider: "openai", Model: "gpt-4o", PriceInput: 0.005, PriceOutput: 0.015},
		{Provider: "openai", Model: "gpt-4o-mini", PriceInput: 0.00015, PriceOutput: 0.0006},
		{Provider: "openai", Model: "gpt-4-turbo", PriceInput: 0.01, PriceOutput: 0.03},
		{Provider: "openai", Model: "gpt-3.5-turbo", PriceInput: 0.0005, PriceOutput: 0.0015},
		// Claude
		{Provider: "claude", Model: "claude-3-5-sonnet-20241022", PriceInput: 0.003, PriceOutput: 0.015},
		{Provider: "claude", Model: "claude-3-opus-20240229", PriceInput: 0.015, PriceOutput: 0.075},
		{Provider: "claude", Model: "claude-3-haiku-20240307", PriceInput: 0.00025, PriceOutput: 0.00125},
		// Gemini
		{Provider: "gemini", Model: "gemini-1.5-pro", PriceInput: 0.00125, PriceOutput: 0.005},
		{Provider: "gemini", Model: "gemini-1.5-flash", PriceInput: 0.000075, PriceOutput: 0.0003},
		// 通义千问
		{Provider: "qwen", Model: "qwen-turbo", PriceInput: 0.0008, PriceOutput: 0.002},
		{Provider: "qwen", Model: "qwen-plus", PriceInput: 0.004, PriceOutput: 0.012},
		{Provider: "qwen", Model: "qwen-max", PriceInput: 0.02, PriceOutput: 0.06},
		// 文心一言
		{Provider: "ernie", Model: "ernie-4.0-8k", PriceInput: 0.017, PriceOutput: 0.017},
		{Provider: "ernie", Model: "ernie-3.5-8k", PriceInput: 0.0017, PriceOutput: 0.0017},
		// 智谱 GLM
		{Provider: "glm", Model: "glm-4", PriceInput: 0.014, PriceOutput: 0.014},
		{Provider: "glm", Model: "glm-4-flash", PriceInput: 0.0001, PriceOutput: 0.0001},
	}

	for _, p := range defaults {
		c.SetPrice(p.Provider, p.Model, p.PriceInput, p.PriceOutput)
	}
}

// SetPrice 设置模型价格
func (c *CostCalculator) SetPrice(provider, model string, priceInput, priceOutput float64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := provider + ":" + model
	c.prices[key] = &ModelPrice{
		Provider:    provider,
		Model:       model,
		PriceInput:  priceInput,
		PriceOutput: priceOutput,
	}
}

// GetPrice 获取模型价格
func (c *CostCalculator) GetPrice(provider, model string) *ModelPrice {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := provider + ":" + model
	return c.prices[key]
}

// Calculate 计算成本
func (c *CostCalculator) Calculate(provider, model string, tokensInput, tokensOutput int) float64 {
	price := c.GetPrice(provider, model)
	if price == nil {
		return 0
	}

	inputCost := float64(tokensInput) / 1000 * price.PriceInput
	outputCost := float64(tokensOutput) / 1000 * price.PriceOutput

	return inputCost + outputCost
}

// UpdatePrices 批量更新价格（从配置/数据库）
func (c *CostCalculator) UpdatePrices(prices []ModelPrice) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, p := range prices {
		key := p.Provider + ":" + p.Model
		c.prices[key] = &ModelPrice{
			Provider:    p.Provider,
			Model:       p.Model,
			PriceInput:  p.PriceInput,
			PriceOutput: p.PriceOutput,
		}
	}
}

// CostSummary 成本汇总
type CostSummary struct {
	TotalCost       float64
	TotalTokens     int
	TokensInput     int
	TokensOutput    int
	RequestCount    int
	AvgCostPerReq   float64
	AvgTokensPerReq float64
}

// CostTracker 成本追踪器（用于会话级别的成本统计）
type CostTracker struct {
	calculator *CostCalculator
	mu         sync.Mutex
	summary    CostSummary
}

// NewCostTracker 创建成本追踪器
func NewCostTracker(calculator *CostCalculator) *CostTracker {
	return &CostTracker{
		calculator: calculator,
	}
}

// Track 追踪一次请求的成本
func (t *CostTracker) Track(provider, model string, tokensInput, tokensOutput int) float64 {
	cost := t.calculator.Calculate(provider, model, tokensInput, tokensOutput)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.summary.TotalCost += cost
	t.summary.TokensInput += tokensInput
	t.summary.TokensOutput += tokensOutput
	t.summary.TotalTokens += tokensInput + tokensOutput
	t.summary.RequestCount++

	if t.summary.RequestCount > 0 {
		t.summary.AvgCostPerReq = t.summary.TotalCost / float64(t.summary.RequestCount)
		t.summary.AvgTokensPerReq = float64(t.summary.TotalTokens) / float64(t.summary.RequestCount)
	}

	return cost
}

// Summary 获取成本汇总
func (t *CostTracker) Summary() CostSummary {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.summary
}

// Reset 重置统计
func (t *CostTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.summary = CostSummary{}
}

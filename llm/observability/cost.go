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
		// OpenAI GPT-5.x 系列（2026-04 定价，$/1K tokens）
		{Provider: "openai", Model: "gpt-5.5", PriceInput: 0.005, PriceOutput: 0.03},
		{Provider: "openai", Model: "gpt-5.5-pro", PriceInput: 0.03, PriceOutput: 0.18},
		{Provider: "openai", Model: "gpt-5.4", PriceInput: 0.0025, PriceOutput: 0.015},
		{Provider: "openai", Model: "gpt-5.4-mini", PriceInput: 0.00075, PriceOutput: 0.0045},
		{Provider: "openai", Model: "gpt-5.4-nano", PriceInput: 0.0002, PriceOutput: 0.00125},
		// OpenAI 旧模型（仍可用于回退）
		{Provider: "openai", Model: "gpt-4o", PriceInput: 0.005, PriceOutput: 0.015},
		{Provider: "openai", Model: "gpt-4o-mini", PriceInput: 0.00015, PriceOutput: 0.0006},
		// Anthropic Claude 4.x 系列（2026-04 定价）
		{Provider: "anthropic", Model: "claude-opus-4-7", PriceInput: 0.005, PriceOutput: 0.025},
		{Provider: "anthropic", Model: "claude-sonnet-4-6", PriceInput: 0.003, PriceOutput: 0.015},
		{Provider: "anthropic", Model: "claude-haiku-4-5", PriceInput: 0.001, PriceOutput: 0.005},
		// Google Gemini 系列（2026-04 定价）
		{Provider: "gemini", Model: "gemini-3.1-pro", PriceInput: 0.002, PriceOutput: 0.012},
		{Provider: "gemini", Model: "gemini-3.1-flash-lite", PriceInput: 0.00025, PriceOutput: 0.0015},
		{Provider: "gemini", Model: "gemini-2.5-pro", PriceInput: 0.00125, PriceOutput: 0.01},
		{Provider: "gemini", Model: "gemini-2.5-flash", PriceInput: 0.00015, PriceOutput: 0.0006},
		// DeepSeek V4 系列（2026-04 定价）
		{Provider: "deepseek", Model: "deepseek-v4-pro", PriceInput: 0.00174, PriceOutput: 0.00348},
		{Provider: "deepseek", Model: "deepseek-v4-flash", PriceInput: 0.00014, PriceOutput: 0.00028},
		// 通义千问 Qwen 系列
		{Provider: "qwen", Model: "qwen3-max-2026-01-23", PriceInput: 0.00036, PriceOutput: 0.00143},
		{Provider: "qwen", Model: "qwen-plus", PriceInput: 0.00012, PriceOutput: 0.00029},
		{Provider: "qwen", Model: "qwen3-coder-next", PriceInput: 0.0002, PriceOutput: 0.0015},
		// 智谱 GLM 系列
		{Provider: "glm", Model: "glm-5.1", PriceInput: 0.00174, PriceOutput: 0.00696},
		{Provider: "glm", Model: "glm-4", PriceInput: 0.014, PriceOutput: 0.014},
		{Provider: "glm", Model: "glm-4-flash", PriceInput: 0.0001, PriceOutput: 0.0001},
		// xAI Grok 系列
		{Provider: "grok", Model: "grok-4.20", PriceInput: 0.005, PriceOutput: 0.015},
		// MiniMax 系列
		{Provider: "minimax", Model: "MiniMax-M2.7", PriceInput: 0.0003, PriceOutput: 0.0009},
		// Mistral 系列
		{Provider: "mistral", Model: "mistral-large-latest", PriceInput: 0.002, PriceOutput: 0.006},
		// 字节豆包系列
		{Provider: "doubao", Model: "Doubao-1.5-pro-256k", PriceInput: 0.0008, PriceOutput: 0.002},
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



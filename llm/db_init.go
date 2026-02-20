package llm

import (
	"fmt"

	"gorm.io/gorm"
)

// InitDatabase 初始化多提供者支持的数据库计划
// 支持: PostgreSQL, MySQL, SQLite, SQL 服务器
func InitDatabase(db *gorm.DB) error {
	// 自动迁移所有表格
	err := db.AutoMigrate(
		&LLMProvider{},
		&LLMModel{},
		&LLMProviderModel{},
		&LLMProviderAPIKey{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	// 创建索引( GORM 通过 struct 标记自动处理)
	// 但是如果需要我们可以加入自定义索引
	
	return nil
}

// 种子实例Data种子实例数据 有50+主流模式 (2026)
// 这是可选的,只应用于发展
func SeedExampleData(db *gorm.DB) error {
	// 检查数据是否存在
	var count int64
	db.Model(&LLMProvider{}).Count(&count)
	if count > 0 {
		return nil // Data already seeded
	}

	// 种子供应商(13个供应商)
	providers := []LLMProvider{
		{Code: "openai", Name: "OpenAI", Status: LLMProviderStatusActive},
		{Code: "anthropic", Name: "Anthropic (Claude)", Status: LLMProviderStatusActive},
		{Code: "google", Name: "Google (Gemini)", Status: LLMProviderStatusActive},
		{Code: "deepseek", Name: "DeepSeek", Status: LLMProviderStatusActive},
		{Code: "doubao", Name: "ByteDance Doubao", Status: LLMProviderStatusActive},
		{Code: "qwen", Name: "Alibaba Qwen", Status: LLMProviderStatusActive},
		{Code: "glm", Name: "Zhipu GLM", Status: LLMProviderStatusActive},
		{Code: "grok", Name: "xAI Grok", Status: LLMProviderStatusActive},
		{Code: "mistral", Name: "Mistral AI", Status: LLMProviderStatusActive},
		{Code: "hunyuan", Name: "Tencent Hunyuan", Status: LLMProviderStatusActive},
		{Code: "kimi", Name: "Moonshot Kimi", Status: LLMProviderStatusActive},
		{Code: "minimax", Name: "MiniMax", Status: LLMProviderStatusActive},
		{Code: "llama", Name: "Meta Llama", Status: LLMProviderStatusActive},
	}
	
	for _, p := range providers {
		if err := db.Create(&p).Error; err != nil {
			return fmt.Errorf("failed to seed provider %s: %w", p.Code, err)
		}
	}

	// 种子50+主流模式 (2026最新)
	models := []LLMModel{
		// OpenAI (5个型号)
		{ModelName: "gpt-5", DisplayName: "GPT-5", Enabled: true},
		{ModelName: "gpt-5-mini", DisplayName: "GPT-5 Mini", Enabled: true},
		{ModelName: "gpt-5-nano", DisplayName: "GPT-5 Nano", Enabled: true},
		{ModelName: "gpt-4o", DisplayName: "GPT-4o", Enabled: true},
		{ModelName: "gpt-4o-mini", DisplayName: "GPT-4o Mini", Enabled: true},
		
		// 雅典克洛德(6个型号)
		{ModelName: "claude-opus-4.5", DisplayName: "Claude Opus 4.5", Enabled: true},
		{ModelName: "claude-sonnet-4.5", DisplayName: "Claude Sonnet 4.5", Enabled: true},
		{ModelName: "claude-haiku-4.5", DisplayName: "Claude Haiku 4.5", Enabled: true},
		{ModelName: "claude-opus-3.5", DisplayName: "Claude Opus 3.5", Enabled: true},
		{ModelName: "claude-sonnet-3.5", DisplayName: "Claude Sonnet 3.5", Enabled: true},
		{ModelName: "claude-haiku-3.5", DisplayName: "Claude Haiku 3.5", Enabled: true},
		
		// 谷歌双子座 (5个模型)
		{ModelName: "gemini-3-pro", DisplayName: "Gemini 3 Pro", Enabled: true},
		{ModelName: "gemini-3-flash", DisplayName: "Gemini 3 Flash", Enabled: true},
		{ModelName: "gemini-2-pro", DisplayName: "Gemini 2.0 Pro", Enabled: true},
		{ModelName: "gemini-2-flash", DisplayName: "Gemini 2.0 Flash", Enabled: true},
		{ModelName: "gemini-1.5-pro", DisplayName: "Gemini 1.5 Pro", Enabled: true},
		
		// 深搜索( 4个型号)
		{ModelName: "deepseek-v3.1", DisplayName: "DeepSeek V3.1", Enabled: true},
		{ModelName: "deepseek-reasoner", DisplayName: "DeepSeek Reasoner", Enabled: true},
		{ModelName: "deepseek-r1", DisplayName: "DeepSeek R1", Enabled: true},
		{ModelName: "deepseek-coder", DisplayName: "DeepSeek Coder", Enabled: true},
		
		// Doubao(4个型号)
		{ModelName: "doubao-1.5-pro", DisplayName: "Doubao 1.5 Pro", Enabled: true},
		{ModelName: "doubao-1.5-lite", DisplayName: "Doubao 1.5 Lite", Enabled: true},
		{ModelName: "doubao-seed-1.8", DisplayName: "Doubao Seed 1.8", Enabled: true},
		{ModelName: "doubao-ui-tars", DisplayName: "Doubao UI TARS", Enabled: true},
		
		// Quen (5个型号)
		{ModelName: "qwen3-235b", DisplayName: "Qwen3 235B", Enabled: true},
		{ModelName: "qwen3-30b", DisplayName: "Qwen3 30B", Enabled: true},
		{ModelName: "qwen3-8b", DisplayName: "Qwen3 8B", Enabled: true},
		{ModelName: "qwen2.5-72b", DisplayName: "Qwen2.5 72B", Enabled: true},
		{ModelName: "qwen2.5-coder", DisplayName: "Qwen2.5 Coder", Enabled: true},
		
		// GLM (4个型号)
		{ModelName: "glm-z1-9b", DisplayName: "GLM-Z1 9B", Enabled: true},
		{ModelName: "glm-4.5-air", DisplayName: "GLM-4.5 Air", Enabled: true},
		{ModelName: "glm-4-plus", DisplayName: "GLM-4 Plus", Enabled: true},
		{ModelName: "glm-4-flash", DisplayName: "GLM-4 Flash", Enabled: true},
		
		// 格罗克(3个型号)
		{ModelName: "grok-4.1", DisplayName: "Grok 4.1", Enabled: true},
		{ModelName: "grok-4.1-fast", DisplayName: "Grok 4.1 Fast", Enabled: true},
		{ModelName: "grok-beta", DisplayName: "Grok Beta", Enabled: true},
		
		// 米斯特拉尔(4个型号)
		{ModelName: "mistral-large-2", DisplayName: "Mistral Large 2", Enabled: true},
		{ModelName: "mistral-medium", DisplayName: "Mistral Medium", Enabled: true},
		{ModelName: "mistral-small", DisplayName: "Mistral Small", Enabled: true},
		{ModelName: "codestral", DisplayName: "Codestral", Enabled: true},
		
		// 湖南(3个型号)
		{ModelName: "hunyuan-pro", DisplayName: "Hunyuan Pro", Enabled: true},
		{ModelName: "hunyuan-lite", DisplayName: "Hunyuan Lite", Enabled: true},
		{ModelName: "hunyuan-turbo", DisplayName: "Hunyuan Turbo", Enabled: true},
		
		// Kimi( 3个型号)
		{ModelName: "kimi-k1.5", DisplayName: "Kimi K1.5", Enabled: true},
		{ModelName: "kimi-k1", DisplayName: "Kimi K1", Enabled: true},
		{ModelName: "moonshot-v1-128k", DisplayName: "Moonshot V1 128K", Enabled: true},
		
		// MiniMax(2个型号)
		{ModelName: "minimax-abab6.5", DisplayName: "MiniMax abab6.5", Enabled: true},
		{ModelName: "minimax-abab6", DisplayName: "MiniMax abab6", Enabled: true},
		
		// (4个型号)
		{ModelName: "llama-4-scout", DisplayName: "Llama 4 Scout", Enabled: true},
		{ModelName: "llama-3.3-70b", DisplayName: "Llama 3.3 70B", Enabled: true},
		{ModelName: "llama-3.1-405b", DisplayName: "Llama 3.1 405B", Enabled: true},
		{ModelName: "llama-3.1-8b", DisplayName: "Llama 3.1 8B", Enabled: true},
	}
	
	for _, m := range models {
		if err := db.Create(&m).Error; err != nil {
			return fmt.Errorf("failed to seed model %s: %w", m.ModelName, err)
		}
	}

	// 有2026个定价和上下文窗口的种子提供者-模型绘图
	providerModels := []LLMProviderModel{
		// OpenAI( 5个型号)
		{ModelID: 1, ProviderID: 1, RemoteModelName: "gpt-5", PriceInput: 0.00125, PriceCompletion: 0.01, MaxTokens: 272000, Priority: 100, Enabled: true},
		{ModelID: 2, ProviderID: 1, RemoteModelName: "gpt-5-mini", PriceInput: 0.00025, PriceCompletion: 0.002, MaxTokens: 272000, Priority: 100, Enabled: true},
		{ModelID: 3, ProviderID: 1, RemoteModelName: "gpt-5-nano", PriceInput: 0.0001, PriceCompletion: 0.0005, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 4, ProviderID: 1, RemoteModelName: "gpt-4o", PriceInput: 0.0025, PriceCompletion: 0.01, MaxTokens: 128000, Priority: 90, Enabled: true},
		{ModelID: 5, ProviderID: 1, RemoteModelName: "gpt-4o-mini", PriceInput: 0.00015, PriceCompletion: 0.0006, MaxTokens: 128000, Priority: 90, Enabled: true},
		
		// (原始内容存档于2018-03-26) (英语). Anthropic Claude (6 model).
		{ModelID: 6, ProviderID: 2, RemoteModelName: "claude-opus-4.5-20260105", PriceInput: 0.005, PriceCompletion: 0.025, MaxTokens: 1000000, Priority: 100, Enabled: true},
		{ModelID: 7, ProviderID: 2, RemoteModelName: "claude-sonnet-4.5-20260105", PriceInput: 0.003, PriceCompletion: 0.015, MaxTokens: 1000000, Priority: 100, Enabled: true},
		{ModelID: 8, ProviderID: 2, RemoteModelName: "claude-haiku-4.5-20260105", PriceInput: 0.001, PriceCompletion: 0.005, MaxTokens: 1000000, Priority: 100, Enabled: true},
		{ModelID: 9, ProviderID: 2, RemoteModelName: "claude-opus-3.5-20250101", PriceInput: 0.015, PriceCompletion: 0.075, MaxTokens: 200000, Priority: 90, Enabled: true},
		{ModelID: 10, ProviderID: 2, RemoteModelName: "claude-sonnet-3.5-20250101", PriceInput: 0.003, PriceCompletion: 0.015, MaxTokens: 200000, Priority: 90, Enabled: true},
		{ModelID: 11, ProviderID: 2, RemoteModelName: "claude-haiku-3.5-20250101", PriceInput: 0.0008, PriceCompletion: 0.004, MaxTokens: 200000, Priority: 90, Enabled: true},
		
		// Google双子座(5个模型)
		{ModelID: 12, ProviderID: 3, RemoteModelName: "gemini-3.0-pro", PriceInput: 0.00125, PriceCompletion: 0.01, MaxTokens: 1000000, Priority: 100, Enabled: true},
		{ModelID: 13, ProviderID: 3, RemoteModelName: "gemini-3.0-flash", PriceInput: 0.0002, PriceCompletion: 0.001, MaxTokens: 1000000, Priority: 100, Enabled: true},
		{ModelID: 14, ProviderID: 3, RemoteModelName: "gemini-2.0-pro", PriceInput: 0.00125, PriceCompletion: 0.005, MaxTokens: 1000000, Priority: 90, Enabled: true},
		{ModelID: 15, ProviderID: 3, RemoteModelName: "gemini-2.0-flash", PriceInput: 0.0001, PriceCompletion: 0.0004, MaxTokens: 1000000, Priority: 90, Enabled: true},
		{ModelID: 16, ProviderID: 3, RemoteModelName: "gemini-1.5-pro", PriceInput: 0.00125, PriceCompletion: 0.005, MaxTokens: 2000000, Priority: 80, Enabled: true},
		
		// ==========(4个型号)
		{ModelID: 17, ProviderID: 4, RemoteModelName: "deepseek-chat", PriceInput: 0.00014, PriceCompletion: 0.00028, MaxTokens: 64000, Priority: 100, Enabled: true},
		{ModelID: 18, ProviderID: 4, RemoteModelName: "deepseek-reasoner", PriceInput: 0.00055, PriceCompletion: 0.0022, MaxTokens: 64000, Priority: 100, Enabled: true},
		{ModelID: 19, ProviderID: 4, RemoteModelName: "deepseek-r1", PriceInput: 0.00055, PriceCompletion: 0.0022, MaxTokens: 64000, Priority: 100, Enabled: true},
		{ModelID: 20, ProviderID: 4, RemoteModelName: "deepseek-coder", PriceInput: 0.00014, PriceCompletion: 0.00028, MaxTokens: 64000, Priority: 100, Enabled: true},
		
		// Doubao(4个型号)
		{ModelID: 21, ProviderID: 5, RemoteModelName: "Doubao-1.5-pro-32k", PriceInput: 0.00011, PriceCompletion: 0.00028, MaxTokens: 32000, Priority: 100, Enabled: true},
		{ModelID: 22, ProviderID: 5, RemoteModelName: "Doubao-1.5-lite-32k", PriceInput: 0.00004, PriceCompletion: 0.00008, MaxTokens: 32000, Priority: 100, Enabled: true},
		{ModelID: 23, ProviderID: 5, RemoteModelName: "Doubao-seed-1.8-32k", PriceInput: 0.00014, PriceCompletion: 0.00035, MaxTokens: 32000, Priority: 100, Enabled: true},
		{ModelID: 24, ProviderID: 5, RemoteModelName: "Doubao-ui-tars-32k", PriceInput: 0.00008, PriceCompletion: 0.00016, MaxTokens: 32000, Priority: 100, Enabled: true},
		
		// QQ (5个型号) QQ
		{ModelID: 25, ProviderID: 6, RemoteModelName: "qwen3-235b-instruct", PriceInput: 0.0004, PriceCompletion: 0.0012, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 26, ProviderID: 6, RemoteModelName: "qwen3-30b-instruct", PriceInput: 0.0002, PriceCompletion: 0.0006, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 27, ProviderID: 6, RemoteModelName: "qwen3-8b-instruct", PriceInput: 0.00008, PriceCompletion: 0.00024, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 28, ProviderID: 6, RemoteModelName: "qwen2.5-72b-instruct", PriceInput: 0.0004, PriceCompletion: 0.0012, MaxTokens: 128000, Priority: 90, Enabled: true},
		{ModelID: 29, ProviderID: 6, RemoteModelName: "qwen2.5-coder-32b-instruct", PriceInput: 0.0002, PriceCompletion: 0.0006, MaxTokens: 128000, Priority: 90, Enabled: true},
		
		// GLM(4个型号)
		{ModelID: 30, ProviderID: 7, RemoteModelName: "glm-z1-9b", PriceInput: 0.0001, PriceCompletion: 0.0001, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 31, ProviderID: 7, RemoteModelName: "glm-4.5-air", PriceInput: 0.00001, PriceCompletion: 0.00001, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 32, ProviderID: 7, RemoteModelName: "glm-4-plus", PriceInput: 0.00005, PriceCompletion: 0.00005, MaxTokens: 128000, Priority: 90, Enabled: true},
		{ModelID: 33, ProviderID: 7, RemoteModelName: "glm-4-flash", PriceInput: 0.000001, PriceCompletion: 0.000001, MaxTokens: 128000, Priority: 90, Enabled: true},
		
		// Grok(3个型号)
		{ModelID: 34, ProviderID: 8, RemoteModelName: "grok-4.1", PriceInput: 0.002, PriceCompletion: 0.01, MaxTokens: 131072, Priority: 100, Enabled: true},
		{ModelID: 35, ProviderID: 8, RemoteModelName: "grok-4.1-fast", PriceInput: 0.0005, PriceCompletion: 0.0025, MaxTokens: 131072, Priority: 100, Enabled: true},
		{ModelID: 36, ProviderID: 8, RemoteModelName: "grok-beta", PriceInput: 0.005, PriceCompletion: 0.015, MaxTokens: 131072, Priority: 80, Enabled: true},
		
		// 米斯特拉尔(4个型号)
		{ModelID: 37, ProviderID: 9, RemoteModelName: "mistral-large-2", PriceInput: 0.002, PriceCompletion: 0.006, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 38, ProviderID: 9, RemoteModelName: "mistral-medium", PriceInput: 0.0027, PriceCompletion: 0.0081, MaxTokens: 32000, Priority: 90, Enabled: true},
		{ModelID: 39, ProviderID: 9, RemoteModelName: "mistral-small", PriceInput: 0.0002, PriceCompletion: 0.0006, MaxTokens: 32000, Priority: 90, Enabled: true},
		{ModelID: 40, ProviderID: 9, RemoteModelName: "codestral-latest", PriceInput: 0.0002, PriceCompletion: 0.0006, MaxTokens: 32000, Priority: 100, Enabled: true},
		
		// ==========(3个型号)
		{ModelID: 41, ProviderID: 10, RemoteModelName: "hunyuan-pro", PriceInput: 0.00014, PriceCompletion: 0.00042, MaxTokens: 32000, Priority: 100, Enabled: true},
		{ModelID: 42, ProviderID: 10, RemoteModelName: "hunyuan-lite", PriceInput: 0.000014, PriceCompletion: 0.000042, MaxTokens: 32000, Priority: 100, Enabled: true},
		{ModelID: 43, ProviderID: 10, RemoteModelName: "hunyuan-turbo", PriceInput: 0.00007, PriceCompletion: 0.00021, MaxTokens: 32000, Priority: 100, Enabled: true},
		
		// Kimi(3个型号)
		{ModelID: 44, ProviderID: 11, RemoteModelName: "kimi-k1.5", PriceInput: 0.00014, PriceCompletion: 0.00014, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 45, ProviderID: 11, RemoteModelName: "kimi-k1", PriceInput: 0.00014, PriceCompletion: 0.00014, MaxTokens: 128000, Priority: 90, Enabled: true},
		{ModelID: 46, ProviderID: 11, RemoteModelName: "moonshot-v1-128k", PriceInput: 0.00014, PriceCompletion: 0.00014, MaxTokens: 128000, Priority: 80, Enabled: true},
		
		// 小麦克斯(2个型号)
		{ModelID: 47, ProviderID: 12, RemoteModelName: "abab6.5-chat", PriceInput: 0.00014, PriceCompletion: 0.00014, MaxTokens: 245760, Priority: 100, Enabled: true},
		{ModelID: 48, ProviderID: 12, RemoteModelName: "abab6-chat", PriceInput: 0.00014, PriceCompletion: 0.00014, MaxTokens: 245760, Priority: 90, Enabled: true},
		
		// 拉玛(4个型号)
		{ModelID: 49, ProviderID: 13, RemoteModelName: "llama-4-scout", PriceInput: 0.0, PriceCompletion: 0.0, MaxTokens: 128000, Priority: 100, Enabled: true},
		{ModelID: 50, ProviderID: 13, RemoteModelName: "llama-3.3-70b-instruct", PriceInput: 0.0, PriceCompletion: 0.0, MaxTokens: 128000, Priority: 90, Enabled: true},
		{ModelID: 51, ProviderID: 13, RemoteModelName: "llama-3.1-405b-instruct", PriceInput: 0.0, PriceCompletion: 0.0, MaxTokens: 128000, Priority: 80, Enabled: true},
		{ModelID: 52, ProviderID: 13, RemoteModelName: "llama-3.1-8b-instruct", PriceInput: 0.0, PriceCompletion: 0.0, MaxTokens: 128000, Priority: 80, Enabled: true},
	}
	
	for _, pm := range providerModels {
		if err := db.Create(&pm).Error; err != nil {
			return fmt.Errorf("failed to seed provider model: %w", err)
		}
	}

	// 种子 API 密钥( 例如 - 替换为真实密钥)
	apiKeys := []LLMProviderAPIKey{
		{ProviderID: 1, APIKey: "sk-example-openai-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 3500, RateLimitRPD: 200000},
		{ProviderID: 2, APIKey: "sk-ant-example-anthropic-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 3, APIKey: "AIza-example-google-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 4, APIKey: "sk-example-deepseek-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 5, APIKey: "your-ark-doubao-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 6, APIKey: "sk-example-qwen-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 7, APIKey: "example-glm-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 8, APIKey: "xai-example-grok-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 9, APIKey: "example-mistral-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 10, APIKey: "example-hunyuan-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 11, APIKey: "example-kimi-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 12, APIKey: "example-minimax-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
		{ProviderID: 13, APIKey: "example-llama-key", Label: "主账号", Priority: 10, Weight: 100, Enabled: false, RateLimitRPM: 0, RateLimitRPD: 0},
	}
	
	for _, key := range apiKeys {
		if err := db.Create(&key).Error; err != nil {
			return fmt.Errorf("failed to seed API key: %w", err)
		}
	}

	return nil
}

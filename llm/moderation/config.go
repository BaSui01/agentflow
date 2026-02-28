package moderation

import (
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
)

// OpenAIConfig 配置 OpenAI 内容审核提供者.
// 嵌入 providers.BaseProviderConfig 以复用 APIKey、BaseURL、Model、Timeout 字段。
type OpenAIConfig struct {
	providers.BaseProviderConfig `yaml:",inline"`
}

// DefaultOpenAIConfig 返回默认 OpenAI 内容审核配置.
func DefaultOpenAIConfig() OpenAIConfig {
	return OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "omni-moderation-latest",
			Timeout: 30 * time.Second,
		},
	}
}

package anthropicofficial

import (
	"net/http"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicsdkoption "github.com/anthropics/anthropic-sdk-go/option"

	"github.com/BaSui01/agentflow/llm/providers"
)

// NewClient builds an official Anthropic Go SDK client using AgentFlow provider config.
func NewClient(cfg providers.ClaudeConfig, apiKey string, httpClient *http.Client) anthropicsdk.Client {
	opts := make([]anthropicsdkoption.RequestOption, 0, 4)
	if trimmed := strings.TrimSpace(apiKey); trimmed != "" {
		opts = append(opts, anthropicsdkoption.WithAPIKey(trimmed))
	}
	if trimmed := strings.TrimSpace(cfg.BaseURL); trimmed != "" {
		opts = append(opts, anthropicsdkoption.WithBaseURL(strings.TrimRight(trimmed, "/")+"/"))
	}
	if httpClient != nil {
		opts = append(opts, anthropicsdkoption.WithHTTPClient(httpClient))
	}
	opts = append(opts, anthropicsdkoption.WithMaxRetries(0))
	return anthropicsdk.NewClient(opts...)
}

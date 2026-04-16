package openaiofficial

import (
	"net/http"
	"strings"

	openaisdk "github.com/openai/openai-go/v3"
	openaisdkoption "github.com/openai/openai-go/v3/option"

	"github.com/BaSui01/agentflow/llm/providers"
)

// NewClient builds an official OpenAI Go SDK client using AgentFlow provider config.
func NewClient(cfg providers.OpenAIConfig, apiKey string, httpClient *http.Client) openaisdk.Client {
	opts := make([]openaisdkoption.RequestOption, 0, 5)
	if trimmed := strings.TrimSpace(apiKey); trimmed != "" {
		opts = append(opts, openaisdkoption.WithAPIKey(trimmed))
	}
	if baseURL := normalizeBaseURL(cfg.BaseURL); baseURL != "" {
		opts = append(opts, openaisdkoption.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, openaisdkoption.WithHTTPClient(httpClient))
	}
	opts = append(opts, openaisdkoption.WithMaxRetries(0))
	if trimmed := strings.TrimSpace(cfg.Organization); trimmed != "" {
		opts = append(opts, openaisdkoption.WithOrganization(trimmed))
	}
	return openaisdk.NewClient(opts...)
}

func normalizeBaseURL(baseURL string) string {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed + "/"
	}
	return trimmed + "/v1/"
}

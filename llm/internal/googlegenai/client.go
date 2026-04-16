package googlegenai

import (
	"context"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

type ClientConfig struct {
	APIKey     string
	BaseURL    string
	ProjectID  string
	Region     string
	AuthType   string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type bearerTransport struct {
	base  http.RoundTripper
	token string
}

func (t *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(clone)
}

func NewClient(ctx context.Context, cfg ClientConfig) (*genai.Client, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = tlsutil.SecureHTTPClient(timeout)
	} else if timeout > 0 {
		httpClient.Timeout = timeout
	}
	token := strings.TrimSpace(cfg.APIKey)
	if strings.EqualFold(strings.TrimSpace(cfg.AuthType), "oauth") && token != "" {
		base := httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		httpClient.Transport = &bearerTransport{
			base:  base,
			token: token,
		}
		token = ""
	}

	clientCfg := &genai.ClientConfig{
		HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: strings.TrimSpace(cfg.BaseURL),
		},
	}

	if strings.TrimSpace(cfg.ProjectID) != "" {
		clientCfg.Backend = genai.BackendVertexAI
		clientCfg.Project = strings.TrimSpace(cfg.ProjectID)
		clientCfg.Location = strings.TrimSpace(cfg.Region)
		if clientCfg.Location == "" {
			clientCfg.Location = "us-central1"
		}
	} else {
		clientCfg.Backend = genai.BackendGeminiAPI
		clientCfg.APIKey = token
		if strings.EqualFold(strings.TrimSpace(cfg.AuthType), "oauth") && clientCfg.APIKey == "" {
			clientCfg.APIKey = strings.TrimSpace(cfg.APIKey)
		}
	}

	return genai.NewClient(ctx, clientCfg)
}

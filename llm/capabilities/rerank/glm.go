package rerank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// GLMConfig 配置智谱 Rerank 提供者.
type GLMConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// GLMProvider 实现智谱 Rerank.
type GLMProvider struct {
	cfg    GLMConfig
	client *http.Client
}

// NewGLMProvider 创建新的 GLM reranker 提供者.
func NewGLMProvider(cfg GLMConfig) *GLMProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://open.bigmodel.cn"
	}
	if cfg.Model == "" {
		cfg.Model = "reranker-v2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &GLMProvider{cfg: cfg, client: tlsutil.SecureHTTPClient(timeout)}
}

func (p *GLMProvider) Name() string      { return "glm-rerank" }
func (p *GLMProvider) MaxDocuments() int { return 1000 }

type glmRerankRequest struct {
	Model           string   `json:"model"`
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents,omitempty"`
}

type glmRerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
		Document       *struct {
			Text string `json:"text"`
		} `json:"document,omitempty"`
	} `json:"results"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// Rerank 使用智谱 Rerank API 重排序文档.
func (p *GLMProvider) Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	docs := make([]string, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = d.Text
	}

	body := glmRerankRequest{
		Model:           model,
		Query:           req.Query,
		Documents:       docs,
		TopN:            req.TopN,
		ReturnDocuments: req.ReturnDocuments,
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/api/paas/v4/rerank",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("glm rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("glm rerank error: status=%d body=%s", resp.StatusCode, string(b))
	}

	var gResp glmRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return nil, fmt.Errorf("failed to decode glm response: %w", err)
	}

	results := make([]RerankResult, len(gResp.Results))
	for i, r := range gResp.Results {
		results[i] = RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		}
		if r.Document != nil {
			results[i].Document = r.Document.Text
		}
	}

	return &RerankResponse{
		ID:        gResp.ID,
		Provider:  p.Name(),
		Model:     model,
		Results:   results,
		Usage:     RerankUsage{TotalTokens: gResp.Usage.TotalTokens},
		CreatedAt: time.Now(),
	}, nil
}

// RerankSimple 是简单重排序的便捷方法.
func (p *GLMProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	docs := make([]Document, len(documents))
	for i, d := range documents {
		docs[i] = Document{Text: d}
	}
	resp, err := p.Rerank(ctx, &RerankRequest{Query: query, Documents: docs, TopN: topN})
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}

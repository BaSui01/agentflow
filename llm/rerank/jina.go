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

	"github.com/BaSui01/agentflow/internal/tlsutil"
)
type JinaProvider struct {
	cfg    JinaConfig
	client *http.Client
}

// NewJinaProvider 创建新的 Jina reranker 提供者.
func NewJinaProvider(cfg JinaConfig) *JinaProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.jina.ai"
	}
	if cfg.Model == "" {
		cfg.Model = "jina-reranker-v2-base-multilingual"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &JinaProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
	}
}

func (p *JinaProvider) Name() string      { return "jina-rerank" }
func (p *JinaProvider) MaxDocuments() int { return 1024 }

type jinaRerankRequest struct {
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	Model           string   `json:"model"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents,omitempty"`
}

type jinaRerankResponse struct {
	Model   string `json:"model"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
		Document       *struct {
			Text string `json:"text"`
		} `json:"document,omitempty"`
	} `json:"results"`
	Usage struct {
		TotalTokens  int `json:"total_tokens"`
		PromptTokens int `json:"prompt_tokens"`
	} `json:"usage"`
}

// 重新排序使用Jina AI的文件重新排序.
func (p *JinaProvider) Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	docs := make([]string, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = d.Text
	}

	body := jinaRerankRequest{
		Query:           req.Query,
		Documents:       docs,
		Model:           model,
		TopN:            req.TopN,
		ReturnDocuments: req.ReturnDocuments,
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v1/rerank",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("jina rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("jina rerank error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var jResp jinaRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&jResp); err != nil {
		return nil, fmt.Errorf("failed to decode jina response: %w", err)
	}

	results := make([]RerankResult, len(jResp.Results))
	for i, r := range jResp.Results {
		results[i] = RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		}
		if r.Document != nil {
			results[i].Document = Document{Text: r.Document.Text}
		}
		if r.Index < len(req.Documents) {
			results[i].Document.ID = req.Documents[r.Index].ID
		}
	}

	return &RerankResponse{
		Provider: p.Name(),
		Model:    jResp.Model,
		Results:  results,
		Usage: RerankUsage{
			TotalTokens: jResp.Usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// RerankSimple是简单的再排的一种方便方法.
func (p *JinaProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	docs := make([]Document, len(documents))
	for i, d := range documents {
		docs[i] = Document{Text: d}
	}

	resp, err := p.Rerank(ctx, &RerankRequest{
		Query:     query,
		Documents: docs,
		TopN:      topN,
	})
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}

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

// QwenConfig 配置阿里云百炼 Rerank 提供者.
type QwenConfig struct {
	APIKey  string        `json:"api_key" yaml:"api_key"`
	BaseURL string        `json:"base_url" yaml:"base_url"`
	Model   string        `json:"model,omitempty" yaml:"model,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

// QwenProvider 实现阿里云百炼 Rerank.
type QwenProvider struct {
	cfg    QwenConfig
	client *http.Client
}

// NewQwenProvider 创建新的 Qwen reranker 提供者.
func NewQwenProvider(cfg QwenConfig) *QwenProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://dashscope.aliyuncs.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gte-rerank"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &QwenProvider{cfg: cfg, client: tlsutil.SecureHTTPClient(timeout)}
}

func (p *QwenProvider) Name() string      { return "qwen-rerank" }
func (p *QwenProvider) MaxDocuments() int { return 1000 }

type qwenRerankRequest struct {
	Model      string          `json:"model"`
	Input      qwenRerankInput `json:"input"`
	Parameters struct {
		TopN            int  `json:"top_n,omitempty"`
		ReturnDocuments bool `json:"return_documents,omitempty"`
	} `json:"parameters,omitempty"`
}

type qwenRerankInput struct {
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
}

type qwenRerankResponse struct {
	Output struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
			Document       *struct {
				Text string `json:"text"`
			} `json:"document,omitempty"`
		} `json:"results"`
	} `json:"output"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
	RequestID string `json:"request_id"`
}

// Rerank 使用阿里云百炼 Rerank API 重排序文档.
func (p *QwenProvider) Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	docs := make([]string, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = d.Text
	}

	body := qwenRerankRequest{
		Model: model,
		Input: qwenRerankInput{Query: req.Query, Documents: docs},
	}
	body.Parameters.TopN = req.TopN
	body.Parameters.ReturnDocuments = req.ReturnDocuments

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/api/v1/services/rerank/text-rerank/text-rerank",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("qwen rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("qwen rerank error: status=%d body=%s", resp.StatusCode, string(b))
	}

	var qResp qwenRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&qResp); err != nil {
		return nil, fmt.Errorf("failed to decode qwen response: %w", err)
	}

	results := make([]RerankResult, len(qResp.Output.Results))
	for i, r := range qResp.Output.Results {
		results[i] = RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		}
		if r.Document != nil {
			results[i].Document = r.Document.Text
		}
	}

	return &RerankResponse{
		ID:       qResp.RequestID,
		Provider: p.Name(),
		Model:    model,
		Results:  results,
		Usage:    RerankUsage{TotalTokens: qResp.Usage.TotalTokens},
		CreatedAt: time.Now(),
	}, nil
}

// RerankSimple 是简单重排序的便捷方法.
func (p *QwenProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
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

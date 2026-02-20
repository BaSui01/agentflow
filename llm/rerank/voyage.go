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
)

// Voyage Provider执行重排,使用Voyage AI的API.
type VoyageProvider struct {
	cfg    VoyageConfig
	client *http.Client
}

// NewVoyage Provider创建了一个新的Voyage reranker供应商.
func NewVoyageProvider(cfg VoyageConfig) *VoyageProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.voyageai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "rerank-2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &VoyageProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *VoyageProvider) Name() string      { return "voyage-rerank" }
func (p *VoyageProvider) MaxDocuments() int { return 1000 }

type voyageRerankRequest struct {
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	Model           string   `json:"model"`
	TopK            int      `json:"top_k,omitempty"`
	ReturnDocuments bool     `json:"return_documents,omitempty"`
	Truncation      bool     `json:"truncation,omitempty"`
}

type voyageRerankResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
		Document       string  `json:"document,omitempty"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

// 重新排序使用Voyage AI的文档重新排序.
func (p *VoyageProvider) Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	docs := make([]string, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = d.Text
	}

	body := voyageRerankRequest{
		Query:           req.Query,
		Documents:       docs,
		Model:           model,
		TopK:            req.TopN,
		ReturnDocuments: req.ReturnDocuments,
		Truncation:      true,
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
		return nil, fmt.Errorf("voyage rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("voyage rerank error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var vResp voyageRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&vResp); err != nil {
		return nil, fmt.Errorf("failed to decode voyage response: %w", err)
	}

	results := make([]RerankResult, len(vResp.Data))
	for i, r := range vResp.Data {
		results[i] = RerankResult{
			Index:          r.Index,
			RelevanceScore: r.RelevanceScore,
		}
		if r.Document != "" {
			results[i].Document = Document{Text: r.Document}
		}
		if r.Index < len(req.Documents) {
			results[i].Document.ID = req.Documents[r.Index].ID
		}
	}

	return &RerankResponse{
		Provider: p.Name(),
		Model:    vResp.Model,
		Results:  results,
		Usage: RerankUsage{
			TotalTokens: vResp.Usage.TotalTokens,
		},
		CreatedAt: time.Now(),
	}, nil
}

// RerankSimple是简单的再排的一种方便方法.
func (p *VoyageProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
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

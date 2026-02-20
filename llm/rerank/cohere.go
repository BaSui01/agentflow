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

// Cohere Provider执行重排 使用 Cohere API.
type CohereProvider struct {
	cfg    CohereConfig
	client *http.Client
}

// NewCohereProvider 创建新的 Cohere reranker 提供者.
func NewCohereProvider(cfg CohereConfig) *CohereProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.cohere.ai"
	}
	if cfg.Model == "" {
		cfg.Model = "rerank-v3.5"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &CohereProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *CohereProvider) Name() string      { return "cohere-rerank" }
func (p *CohereProvider) MaxDocuments() int { return 1000 }

type cohereRerankRequest struct {
	Query           string   `json:"query"`
	Documents       []string `json:"documents"`
	Model           string   `json:"model"`
	TopN            int      `json:"top_n,omitempty"`
	ReturnDocuments bool     `json:"return_documents,omitempty"`
	MaxChunksPerDoc int      `json:"max_chunks_per_doc,omitempty"`
}

type cohereRerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
		Document       *struct {
			Text string `json:"text"`
		} `json:"document,omitempty"`
	} `json:"results"`
	Meta struct {
		BilledUnits struct {
			SearchUnits int `json:"search_units"`
		} `json:"billed_units"`
	} `json:"meta"`
}

// 使用 Cohere 对文档进行重新排序 。
func (p *CohereProvider) Rerank(ctx context.Context, req *RerankRequest) (*RerankResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}

	docs := make([]string, len(req.Documents))
	for i, d := range req.Documents {
		docs[i] = d.Text
	}

	body := cohereRerankRequest{
		Query:           req.Query,
		Documents:       docs,
		Model:           model,
		TopN:            req.TopN,
		ReturnDocuments: req.ReturnDocuments,
		MaxChunksPerDoc: req.MaxChunksPerDoc,
	}

	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		strings.TrimRight(p.cfg.BaseURL, "/")+"/v2/rerank",
		bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cohere rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cohere rerank error: status=%d body=%s", resp.StatusCode, string(body))
	}

	var cResp cohereRerankResponse
	if err := json.NewDecoder(resp.Body).Decode(&cResp); err != nil {
		return nil, fmt.Errorf("failed to decode cohere response: %w", err)
	}

	results := make([]RerankResult, len(cResp.Results))
	for i, r := range cResp.Results {
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
		ID:       cResp.ID,
		Provider: p.Name(),
		Model:    model,
		Results:  results,
		Usage: RerankUsage{
			SearchUnits: cResp.Meta.BilledUnits.SearchUnits,
		},
		CreatedAt: time.Now(),
	}, nil
}

// RerankSimple是简单的再排的一种方便方法.
func (p *CohereProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
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

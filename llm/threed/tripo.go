package threed

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

// TripoProvider implements 3D generation using Tripo3D API.
type TripoProvider struct {
	cfg    TripoConfig
	client *http.Client
}

// NewTripoProvider creates a new Tripo3D provider.
func NewTripoProvider(cfg TripoConfig) *TripoProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.tripo3d.ai/v2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 600 * time.Second
	}
	return &TripoProvider{
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *TripoProvider) Name() string { return "tripo3d" }

type tripoRequest struct {
	Type         string     `json:"type"`
	Prompt       string     `json:"prompt,omitempty"`
	File         *tripoFile `json:"file,omitempty"`
	ModelVersion string     `json:"model_version,omitempty"`
}

type tripoFile struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	Data string `json:"file_token,omitempty"`
}

type tripoResponse struct {
	Code int `json:"code"`
	Data struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
		Output struct {
			Model         string `json:"model"`
			PbrModel      string `json:"pbr_model"`
			RenderedImage string `json:"rendered_image"`
		} `json:"output"`
	} `json:"data"`
}

// Generate creates 3D models using Tripo3D API.
func (p *TripoProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	var taskID string
	var err error

	if req.Image != "" || req.ImageURL != "" || len(req.Images) > 0 {
		taskID, err = p.createImageTask(ctx, req)
	} else {
		taskID, err = p.createTextTask(ctx, req)
	}
	if err != nil {
		return nil, err
	}

	result, err := p.pollTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    p.cfg.Model,
		Models: []ModelData{{
			ID:           taskID,
			URL:          result.Data.Output.Model,
			Format:       "glb",
			ThumbnailURL: result.Data.Output.RenderedImage,
		}},
		Usage: ThreeDUsage{
			ModelsGenerated: 1,
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *TripoProvider) createTextTask(ctx context.Context, req *GenerateRequest) (string, error) {
	body := tripoRequest{
		Type:         "text_to_model",
		Prompt:       req.Prompt,
		ModelVersion: p.cfg.Model,
	}

	return p.submitTask(ctx, body)
}

func (p *TripoProvider) createImageTask(ctx context.Context, req *GenerateRequest) (string, error) {
	taskType := "image_to_model"
	if len(req.Images) > 1 {
		taskType = "multiview_to_model"
	}

	body := tripoRequest{
		Type:         taskType,
		ModelVersion: p.cfg.Model,
	}

	if req.ImageURL != "" {
		body.File = &tripoFile{Type: "url", URL: req.ImageURL}
	} else if req.Image != "" {
		body.File = &tripoFile{Type: "base64", Data: req.Image}
	}

	return p.submitTask(ctx, body)
}

func (p *TripoProvider) submitTask(ctx context.Context, body tripoRequest) (string, error) {
	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/openapi/task", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("tripo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("tripo error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var tResp tripoResponse
	if err := json.NewDecoder(resp.Body).Decode(&tResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if tResp.Code != 0 {
		return "", fmt.Errorf("tripo error code: %d", tResp.Code)
	}

	return tResp.Data.TaskID, nil
}

func (p *TripoProvider) pollTask(ctx context.Context, taskID string) (*tripoResponse, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			endpoint := fmt.Sprintf("%s/openapi/task/%s", strings.TrimRight(p.cfg.BaseURL, "/"), taskID)
			httpReq, _ := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var tResp tripoResponse
			json.NewDecoder(resp.Body).Decode(&tResp)
			resp.Body.Close()

			if tResp.Data.Status == "success" {
				return &tResp, nil
			}
			if tResp.Data.Status == "failed" {
				return nil, fmt.Errorf("tripo generation failed")
			}
		}
	}
}

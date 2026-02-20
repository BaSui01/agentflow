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

	"github.com/BaSui01/agentflow/internal/tlsutil"
)

// MeshyProvider使用Meshy API执行3D生成.
type MeshyProvider struct {
	cfg    MeshyConfig
	client *http.Client
}

// NewMeshyProvider 创建新的 Meshy 3D 提供者.
func NewMeshyProvider(cfg MeshyConfig) *MeshyProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.meshy.ai/v2"
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 600 * time.Second
	}
	return &MeshyProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
	}
}

func (p *MeshyProvider) Name() string { return "meshy" }

type meshyTextTo3DRequest struct {
	Mode           string `json:"mode"`
	Prompt         string `json:"prompt"`
	ArtStyle       string `json:"art_style,omitempty"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
}

type meshyImageTo3DRequest struct {
	ImageURL string `json:"image_url"`
}

type meshyTaskResponse struct {
	Result    string `json:"result"`
	TaskID    string `json:"task_id,omitempty"`
	ID        string `json:"id,omitempty"`
	Status    string `json:"status"`
	ModelURLs struct {
		GLB  string `json:"glb"`
		FBX  string `json:"fbx"`
		OBJ  string `json:"obj"`
		USDZ string `json:"usdz"`
	} `json:"model_urls"`
	ThumbnailURL string `json:"thumbnail_url"`
}

// 生成使用Messy API创建了3D模型.
func (p *MeshyProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	var taskID string
	var err error

	if req.Image != "" || req.ImageURL != "" {
		taskID, err = p.createImageTo3DTask(ctx, req)
	} else {
		taskID, err = p.createTextTo3DTask(ctx, req)
	}
	if err != nil {
		return nil, err
	}

	result, err := p.pollTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	format := req.Format
	if format == "" {
		format = "glb"
	}

	var modelURL string
	switch format {
	case "fbx":
		modelURL = result.ModelURLs.FBX
	case "obj":
		modelURL = result.ModelURLs.OBJ
	case "usdz":
		modelURL = result.ModelURLs.USDZ
	default:
		modelURL = result.ModelURLs.GLB
	}

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    p.cfg.Model,
		Models: []ModelData{{
			ID:           taskID,
			URL:          modelURL,
			Format:       format,
			ThumbnailURL: result.ThumbnailURL,
		}},
		Usage: ThreeDUsage{
			ModelsGenerated: 1,
		},
		CreatedAt: time.Now(),
	}, nil
}

func (p *MeshyProvider) createTextTo3DTask(ctx context.Context, req *GenerateRequest) (string, error) {
	body := meshyTextTo3DRequest{
		Mode:     "preview",
		Prompt:   req.Prompt,
		ArtStyle: req.Style,
	}
	if req.Quality == "high" {
		body.Mode = "refine"
	}

	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/text-to-3d", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("meshy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("meshy error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var mResp meshyTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&mResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	taskID := mResp.TaskID
	if taskID == "" {
		taskID = mResp.Result
	}
	return taskID, nil
}

func (p *MeshyProvider) createImageTo3DTask(ctx context.Context, req *GenerateRequest) (string, error) {
	imageURL := req.ImageURL
	if imageURL == "" && req.Image != "" {
		imageURL = "data:image/png;base64," + req.Image
	}

	body := meshyImageTo3DRequest{ImageURL: imageURL}
	payload, _ := json.Marshal(body)
	endpoint := fmt.Sprintf("%s/image-to-3d", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("meshy request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("meshy error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var mResp meshyTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&mResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	taskID := mResp.TaskID
	if taskID == "" {
		taskID = mResp.Result
	}
	return taskID, nil
}

func (p *MeshyProvider) pollTask(ctx context.Context, taskID string) (*meshyTaskResponse, error) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			endpoint := fmt.Sprintf("%s/text-to-3d/%s", strings.TrimRight(p.cfg.BaseURL, "/"), taskID)
			httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}
			httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

			resp, err := p.client.Do(httpReq)
			if err != nil {
				continue
			}

			var mResp meshyTaskResponse
			json.NewDecoder(resp.Body).Decode(&mResp)
			resp.Body.Close()

			if mResp.Status == "SUCCEEDED" {
				return &mResp, nil
			}
			if mResp.Status == "FAILED" || mResp.Status == "EXPIRED" {
				return nil, fmt.Errorf("meshy generation failed: %s", mResp.Status)
			}
		}
	}
}

package image

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

// KlingProvider 使用可灵 Kling 文生图/图生图 API；与视频共用 api.klingai.com，可共用同一 API Key。
// 流程：POST /kling/v1/images/generations 提交 → GET /kling/v1/images/generations/{task_id} 轮询直至 succeed。
// 参考：https://app.klingai.com/cn/dev/document-api/apiReference/model/imageGeneration
const defaultKlingImageTimeout = 120 * time.Second

const defaultKlingImageBaseURL = "https://api.klingai.com"

const (
	klingImageSubmitPath = "/kling/v1/images/generations"
	klingImageTaskPrefix = "/kling/v1/images/generations/"
)

var klingImageAllowedModels = map[string]struct{}{
	"kling-v1": {}, "kling-v1-5": {}, "kling-v1-6": {},
}

type KlingProvider struct {
	cfg    KlingConfig
	client *http.Client
}

func NewKlingProvider(cfg KlingConfig) *KlingProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultKlingImageBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = "kling-v1-5"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultKlingImageTimeout
	}
	return &KlingProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(cfg.Timeout),
	}
}

func (p *KlingProvider) Name() string { return "kling" }

func (p *KlingProvider) SupportedSizes() []string {
	return []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2", "2:3"}
}

type klingImageSubmitReq struct {
	Prompt         string `json:"prompt"`
	NegativePrompt string `json:"negative_prompt,omitempty"`
	ModelName      string `json:"model_name"`
	AspectRatio    string `json:"aspect_ratio,omitempty"`
	N              int    `json:"n,omitempty"`
}

type klingImageSubmitResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
	} `json:"data"`
}

type klingImageTaskResp struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		TaskResult *struct {
			Images []struct {
				URL string `json:"url"`
			} `json:"images"`
		} `json:"task_result,omitempty"`
	} `json:"data"`
}

func (p *KlingProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	if _, ok := klingImageAllowedModels[model]; !ok {
		model = p.cfg.Model
	}
	aspectRatio := p.sizeToAspect(req.Size)
	n := req.N
	if n <= 0 {
		n = 1
	}
	if n > 4 {
		n = 4
	}

	body := klingImageSubmitReq{
		Prompt:      req.Prompt,
		ModelName:   model,
		AspectRatio: aspectRatio,
		N:           n,
	}
	if req.NegativePrompt != "" {
		body.NegativePrompt = req.NegativePrompt
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("kling image marshal: %w", err)
	}
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	submitURL := base + klingImageSubmitPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, submitURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("kling image request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kling image request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var sub klingImageSubmitResp
	if err := json.Unmarshal(respBody, &sub); err != nil {
		return nil, fmt.Errorf("kling image submit decode: %w", err)
	}
	if sub.Code != 0 && sub.Code != 200 {
		return nil, fmt.Errorf("kling image submit: code=%d msg=%s", sub.Code, sub.Message)
	}
	taskID := sub.Data.TaskID
	if taskID == "" {
		return nil, fmt.Errorf("kling image submit: empty task_id")
	}

	// 轮询任务结果
	taskURL := base + klingImageTaskPrefix + taskID
	for i := 0; i < 60; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}

		getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
		if err != nil {
			continue
		}
		getReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		getReq.Header.Set("Content-Type", "application/json")
		getResp, err := p.client.Do(getReq)
		if err != nil {
			continue
		}
		getBody, _ := io.ReadAll(getResp.Body)
		getResp.Body.Close()

		var task klingImageTaskResp
		if err := json.Unmarshal(getBody, &task); err != nil {
			continue
		}
		if task.Data.TaskStatus == "succeed" || task.Data.TaskStatus == "completed" {
			var images []ImageData
			if task.Data.TaskResult != nil {
				for _, img := range task.Data.TaskResult.Images {
					if img.URL != "" {
						images = append(images, ImageData{URL: img.URL})
					}
				}
			}
			if len(images) == 0 {
				return nil, fmt.Errorf("kling image task succeed but no image url")
			}
			return &GenerateResponse{
				Provider:  p.Name(),
				Model:     model,
				Images:    images,
				Usage:     ImageUsage{ImagesGenerated: len(images)},
				CreatedAt: time.Now(),
			}, nil
		}
		if task.Data.TaskStatus == "failed" || task.Data.TaskStatus == "error" {
			return nil, fmt.Errorf("kling image task failed: %s", task.Message)
		}
	}
	return nil, fmt.Errorf("kling image poll timeout")
}

func (p *KlingProvider) sizeToAspect(size string) string {
	if size == "" {
		return "1:1"
	}
	size = strings.TrimSpace(strings.ToLower(size))
	switch {
	case strings.Contains(size, "1024x1024") || size == "1:1":
		return "1:1"
	case strings.Contains(size, "16:9") || strings.Contains(size, "1920") || strings.Contains(size, "1280x720"):
		return "16:9"
	case strings.Contains(size, "9:16") || strings.Contains(size, "720x1280"):
		return "9:16"
	case strings.Contains(size, "4:3"):
		return "4:3"
	case strings.Contains(size, "3:4"):
		return "3:4"
	case strings.Contains(size, "3:2"):
		return "3:2"
	case strings.Contains(size, "2:3"):
		return "2:3"
	}
	return "1:1"
}

func (p *KlingProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("kling does not support image editing via this API")
}

func (p *KlingProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	return nil, fmt.Errorf("kling does not support image variations via this API")
}

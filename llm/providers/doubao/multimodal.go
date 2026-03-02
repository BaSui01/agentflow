package doubao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage 使用 Doubao 生成图像.
func (p *DoubaoProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/v3/images/generations", req, p.ApplyHeaders)
}

// GenerateVideo 使用火山方舟视频生成.
// Endpoint: POST /api/v3/videos/generations (提交) + GET /api/v3/videos/generations/{id} (轮询)
func (p *DoubaoProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	apiKey := p.ResolveAPIKey(ctx)
	baseURL := strings.TrimRight(p.Cfg.BaseURL, "/")

	// 1. 提交视频生成任务
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v3/videos/generations", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, apiKey)

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var submitResp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	if submitResp.ID == "" {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: "empty video generation id", HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	// 2. 异步轮询
	result, err := providers.Poll[llm.VideoGenerationResponse](ctx, providers.PollConfig{
		Interval:    5 * time.Second,
		MaxAttempts: 120,
	}, func(ctx context.Context) providers.PollResult[llm.VideoGenerationResponse] {
		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v3/videos/generations/"+submitResp.ID, nil)
		if err != nil {
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: err}
		}
		p.ApplyHeaders(pollReq, apiKey)

		pollResp, err := p.Client.Do(pollReq)
		if err != nil {
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: &types.Error{
				Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
			}}
		}
		defer pollResp.Body.Close()

		if pollResp.StatusCode >= 400 {
			msg := providerbase.ReadErrorMessage(pollResp.Body)
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: providerbase.MapHTTPError(pollResp.StatusCode, msg, p.Name())}
		}

		var statusResp struct {
			ID        string `json:"id"`
			Status    string `json:"status"`
			CreatedAt int64  `json:"created_at"`
			Data      []struct {
				URL string `json:"url"`
			} `json:"data,omitempty"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
		}
		if err := json.NewDecoder(pollResp.Body).Decode(&statusResp); err != nil {
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: &types.Error{
				Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
			}}
		}

		switch statusResp.Status {
		case "completed":
			r := &llm.VideoGenerationResponse{ID: statusResp.ID, Created: statusResp.CreatedAt}
			for _, v := range statusResp.Data {
				r.Data = append(r.Data, llm.Video{URL: v.URL})
			}
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Result: r}
		case "failed":
			errMsg := "video generation failed"
			if statusResp.Error != nil {
				errMsg = statusResp.Error.Message
			}
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: &types.Error{
				Code: llm.ErrUpstreamError, Message: errMsg, HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
			}}
		default:
			return providers.PollResult[llm.VideoGenerationResponse]{}
		}
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GenerateAudio 使用 Doubao 生成音频.
func (p *DoubaoProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/v3/audio/speech", req, p.ApplyHeaders)
}

// TranscribeAudio Doubao 不支持音频转录.
func (p *DoubaoProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 Doubao 创建嵌入.
func (p *DoubaoProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/api/v3/embeddings", req, p.ApplyHeaders)
}

// CreateFineTuningJob Doubao 不支持微调.
func (p *DoubaoProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Doubao 不支持微调.
func (p *DoubaoProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Doubao 不支持微调.
func (p *DoubaoProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Doubao 不支持微调.
func (p *DoubaoProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

package qwen

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

// GenerateImage 使用 Qwen Wanx 生成图像.
// Endpoint: POST /compatible-mode/v1/images/generations
// Models: wanx-v1, wanx2.1-t2i-turbo, wanx2.1-t2i-plus
func (p *QwenProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return providerbase.GenerateImageOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/compatible-mode/v1/images/generations", req, p.ApplyHeaders)
}

// GenerateVideo 使用 Qwen 万相视频生成.
// Endpoint: POST /api/v1/services/aigc/video-generation/generation (提交)
//
//	GET /api/v1/tasks/{task_id} (轮询)
//
// Models: wanx2.1-t2v-turbo, wanx2.1-t2v-plus
func (p *QwenProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	apiKey := p.ResolveAPIKey(ctx)
	baseURL := strings.TrimRight(p.Cfg.BaseURL, "/")

	model := req.Model
	if model == "" {
		model = "wanx2.1-t2v-turbo"
	}

	// 1. 提交视频生成任务（百炼异步任务格式）
	submitBody := qwenVideoSubmitRequest{
		Model: model,
		Input: qwenVideoInput{Prompt: req.Prompt},
	}
	if req.Resolution != "" {
		submitBody.Parameters.Size = req.Resolution
	}

	payload, err := json.Marshal(submitBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/services/aigc/video-generation/generation", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, apiKey)
	httpReq.Header.Set("X-DashScope-Async", "enable")

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var submitResp qwenAsyncTaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	taskID := submitResp.Output.TaskID
	if taskID == "" {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: "empty task id", HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	// 2. 异步轮询
	result, err := providers.Poll[llm.VideoGenerationResponse](ctx, providers.PollConfig{
		Interval:    5 * time.Second,
		MaxAttempts: 120,
	}, func(ctx context.Context) providers.PollResult[llm.VideoGenerationResponse] {
		pollReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/tasks/"+taskID, nil)
		if err != nil {
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: err}
		}
		p.ApplyHeaders(pollReq, apiKey)

		pollResp, err := p.Client.Do(pollReq)
		if err != nil {
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: &types.Error{
				Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name(),
			}}
		}
		defer pollResp.Body.Close()

		if pollResp.StatusCode >= 400 {
			msg := providerbase.ReadErrorMessage(pollResp.Body)
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: providerbase.MapHTTPError(pollResp.StatusCode, msg, p.Name())}
		}

		var taskResp qwenAsyncTaskResponse
		if err := json.NewDecoder(pollResp.Body).Decode(&taskResp); err != nil {
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: &types.Error{
				Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
			}}
		}

		switch taskResp.Output.TaskStatus {
		case "SUCCEEDED":
			r := &llm.VideoGenerationResponse{ID: taskResp.Output.TaskID}
			if taskResp.Output.VideoURL != "" {
				r.Data = append(r.Data, llm.Video{URL: taskResp.Output.VideoURL})
			}
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Result: r}
		case "FAILED":
			errMsg := "video generation failed"
			if taskResp.Output.Message != "" {
				errMsg = taskResp.Output.Message
			}
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: &types.Error{
				Code: llm.ErrUpstreamError, Message: errMsg, HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
			}}
		default: // PENDING, RUNNING
			return providers.PollResult[llm.VideoGenerationResponse]{}
		}
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// qwenVideoSubmitRequest 万相视频生成提交请求.
type qwenVideoSubmitRequest struct {
	Model      string         `json:"model"`
	Input      qwenVideoInput `json:"input"`
	Parameters struct {
		Size string `json:"size,omitempty"`
	} `json:"parameters,omitempty"`
}

type qwenVideoInput struct {
	Prompt string `json:"prompt"`
}

// qwenAsyncTaskResponse 百炼异步任务通用响应.
type qwenAsyncTaskResponse struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"` // PENDING, RUNNING, SUCCEEDED, FAILED
		VideoURL   string `json:"video_url,omitempty"`
		Message    string `json:"message,omitempty"`
	} `json:"output"`
	RequestID string `json:"request_id,omitempty"`
}

// GenerateAudio 使用 Qwen TTS 生成音频.
// Endpoint: POST /compatible-mode/v1/audio/speech
// Models: cosyvoice-v1, sambert-v1, qwen-tts
func (p *QwenProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return providerbase.GenerateAudioOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/compatible-mode/v1/audio/speech", req, p.ApplyHeaders)
}

// TranscribeAudio Qwen 不支持音频转录.
func (p *QwenProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio transcription")
}

// CreateEmbedding 使用 Qwen 创建嵌入.
// Endpoint: POST /compatible-mode/v1/embeddings
// Models: text-embedding-v4, text-embedding-v3, text-embedding-v2
func (p *QwenProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.Client, p.Cfg.BaseURL, p.ResolveAPIKey(ctx), p.Name(), "/compatible-mode/v1/embeddings", req, p.ApplyHeaders)
}

// CreateFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Qwen 不支持微调.
func (p *QwenProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Qwen 不支持微调.
func (p *QwenProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
)

// =============================================================================
// 图像生成
// =============================================================================

// GenerateImage 使用 DALL·E / gpt-image-1 从文本提示生成图像.
// Endpoint: POST /v1/images/generations
// Models: dall-e-3, dall-e-2, gpt-image-1
func (p *OpenAIProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	var imageResp llm.ImageGenerationResponse
	if err := p.doJSON(ctx, http.MethodPost, "/v1/images/generations", req, &imageResp); err != nil {
		return nil, err
	}
	return &imageResp, nil
}

// GenerateVideo 使用 OpenAI Sora 生成视频.
// Endpoint: POST /v1/videos/generations (提交) + GET /v1/videos/generations/{id} (轮询)
// Models: sora
func (p *OpenAIProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	// 1. 提交视频生成任务
	var submitResp openaiVideoSubmitResponse
	if err := p.doJSON(ctx, http.MethodPost, "/v1/videos/generations", req, &submitResp); err != nil {
		return nil, err
	}

	if submitResp.ID == "" {
		return nil, &types.Error{
			Code: llm.ErrUpstreamError, Message: "empty video generation id",
			HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
		}
	}

	// 2. 异步轮询等待完成
	result, err := providers.Poll[llm.VideoGenerationResponse](ctx, providers.PollConfig{
		Interval:    5 * time.Second,
		MaxAttempts: 120,
	}, func(ctx context.Context) providers.PollResult[llm.VideoGenerationResponse] {
		var statusResp openaiVideoStatusResponse
		if err := p.doJSON(ctx, http.MethodGet, "/v1/videos/generations/"+submitResp.ID, nil, &statusResp); err != nil {
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: err}
		}
		switch statusResp.Status {
		case "completed":
			resp := &llm.VideoGenerationResponse{
				ID:      statusResp.ID,
				Created: statusResp.CreatedAt,
			}
			for _, v := range statusResp.Data {
				resp.Data = append(resp.Data, llm.Video{URL: v.URL})
			}
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Result: resp}
		case "failed":
			errMsg := "video generation failed"
			if statusResp.Error != nil {
				errMsg = statusResp.Error.Message
			}
			return providers.PollResult[llm.VideoGenerationResponse]{Done: true, Err: &types.Error{
				Code: llm.ErrUpstreamError, Message: errMsg,
				HTTPStatus: http.StatusBadGateway, Provider: p.Name(),
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

// openaiVideoSubmitResponse 表示视频生成提交响应.
type openaiVideoSubmitResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// openaiVideoStatusResponse 表示视频生成状态轮询响应.
type openaiVideoStatusResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"` // pending, processing, completed, failed
	CreatedAt int64  `json:"created_at"`
	Data      []struct {
		URL string `json:"url"`
	} `json:"data,omitempty"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// =============================================================================
// 音频生成和转录
// =============================================================================

// GenerateAudio 使用 TTS 从文本生成音频/语音.
// Endpoint: POST /v1/audio/speech
// Models: tts-1, tts-1-hd, gpt-4o-mini-tts
func (p *OpenAIProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	audioData, err := p.doBytes(ctx, http.MethodPost, "/v1/audio/speech", req)
	if err != nil {
		return nil, err
	}

	return &llm.AudioGenerationResponse{
		Audio: audioData,
	}, nil
}

// TranscribeAudio 使用 Whisper / gpt-4o-transcribe 将音频转为文字.
// Endpoint: POST /v1/audio/transcriptions
// Models: whisper-1, gpt-4o-transcribe, gpt-4o-mini-transcribe
func (p *OpenAIProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	// 创建 multipart/form-data 请求体
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 添加文件
	part, err := writer.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(req.File); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// 添加其他字段
	if err := writer.WriteField("model", req.Model); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}
	if req.Language != "" {
		if err := writer.WriteField("language", req.Language); err != nil {
			return nil, fmt.Errorf("failed to write language field: %w", err)
		}
	}
	if req.Prompt != "" {
		if err := writer.WriteField("prompt", req.Prompt); err != nil {
			return nil, fmt.Errorf("failed to write prompt field: %w", err)
		}
	}
	if req.ResponseFormat != "" {
		if err := writer.WriteField("response_format", req.ResponseFormat); err != nil {
			return nil, fmt.Errorf("failed to write response_format field: %w", err)
		}
	}
	if req.Temperature > 0 {
		if err := writer.WriteField("temperature", fmt.Sprintf("%f", req.Temperature)); err != nil {
			return nil, fmt.Errorf("failed to write temperature field: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize multipart body: %w", err)
	}

	var transcriptionResp llm.AudioTranscriptionResponse
	if err := p.doMultipartJSON(ctx, "/v1/audio/transcriptions", body, writer.FormDataContentType(), &transcriptionResp); err != nil {
		return nil, err
	}

	return &transcriptionResp, nil
}

// =============================================================================
// 嵌入
// =============================================================================

// CreateEmbedding 为给定输入创建嵌入.
// Endpoint: POST /v1/embeddings
// Models: text-embedding-3-small, text-embedding-3-large, text-embedding-ada-002
func (p *OpenAIProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	var embeddingResp llm.EmbeddingResponse
	if err := p.doJSON(ctx, http.MethodPost, "/v1/embeddings", req, &embeddingResp); err != nil {
		return nil, err
	}

	return &embeddingResp, nil
}

// =============================================================================
// 微调
// =============================================================================

// CreateFineTuningJob 创建微调任务.
// Endpoint: POST /v1/fine_tuning/jobs
func (p *OpenAIProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	var job llm.FineTuningJob
	if err := p.doJSON(ctx, http.MethodPost, "/v1/fine_tuning/jobs", req, &job); err != nil {
		return nil, err
	}

	return &job, nil
}

// ListFineTuningJobs 列出微调任务.
// Endpoint: GET /v1/fine_tuning/jobs
func (p *OpenAIProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	var jobsResp struct {
		Data []llm.FineTuningJob `json:"data"`
	}
	if err := p.doJSON(ctx, http.MethodGet, "/v1/fine_tuning/jobs", nil, &jobsResp); err != nil {
		return nil, err
	}

	return jobsResp.Data, nil
}

// GetFineTuningJob 通过 ID 获取微调任务.
// Endpoint: GET /v1/fine_tuning/jobs/{job_id}
func (p *OpenAIProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	var job llm.FineTuningJob
	if err := p.doJSON(ctx, http.MethodGet, "/v1/fine_tuning/jobs/"+jobID, nil, &job); err != nil {
		return nil, err
	}

	return &job, nil
}

// CancelFineTuningJob 取消微调任务.
// Endpoint: POST /v1/fine_tuning/jobs/{job_id}/cancel
func (p *OpenAIProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	httpReq, err := p.newRequest(ctx, http.MethodPost, "/v1/fine_tuning/jobs/"+jobID+"/cancel", nil)
	if err != nil {
		return err
	}
	resp, err := p.do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	return nil
}

func (p *OpenAIProvider) endpoint(path string) string {
	return fmt.Sprintf("%s%s", strings.TrimRight(p.openaiCfg.BaseURL, "/"), path)
}

func (p *OpenAIProvider) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.endpoint(path), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.Provider.ApplyHeaders(req, p.Provider.ResolveAPIKey(ctx))
	return req, nil
}

func (p *OpenAIProvider) do(req *http.Request) (*http.Response, error) {
	resp, err := p.Provider.Client.Do(req)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	return resp, nil
}

func (p *OpenAIProvider) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := p.newRequest(ctx, method, path, reqBody)
	if err != nil {
		return err
	}

	resp, err := p.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	return nil
}

func (p *OpenAIProvider) doBytes(ctx context.Context, method, path string, payload any) ([]byte, error) {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := p.newRequest(ctx, method, path, reqBody)
	if err != nil {
		return nil, err
	}
	resp, err := p.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	return data, nil
}

func (p *OpenAIProvider) doMultipartJSON(ctx context.Context, path string, body io.Reader, contentType string, out any) error {
	req, err := p.newRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := p.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return &types.Error{
			Code:    llm.ErrUpstreamError,
			Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway,
			Retryable: true,
			Provider:  p.Name(),
		}
	}
	return nil
}

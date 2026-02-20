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

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/providers"
)

// =============================================================================
// QQ 图像生成
// =============================================================================

// 生成图像会使用 DALL- E 从文本提示生成图像 。
func (p *OpenAIProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/images/generations", strings.TrimRight(p.cfg.BaseURL, "/"))

	payload, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var imageResp llm.ImageGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&imageResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return &imageResp, nil
}

// GenerateVideo 不被 OpenAI 支持.
func (p *OpenAIProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, &llm.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    "video generation is not supported by OpenAI",
		HTTPStatus: http.StatusNotImplemented,
		Provider:   p.Name(),
	}
}

// =============================================================================
// QQ 音频生成和转录
// =============================================================================

// 生成Audio通过TTS从文本中生成音频/语音.
func (p *OpenAIProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/audio/speech", strings.TrimRight(p.cfg.BaseURL, "/"))

	payload, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	// 读取音频数据
	audioData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return &llm.AudioGenerationResponse{
		Audio: audioData,
	}, nil
}

// 将音频转换为文字使用Whisper.
func (p *OpenAIProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/audio/transcriptions", strings.TrimRight(p.cfg.BaseURL, "/"))

	// 创建多部分形式数据
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
	writer.WriteField("model", req.Model)
	if req.Language != "" {
		writer.WriteField("language", req.Language)
	}
	if req.Prompt != "" {
		writer.WriteField("prompt", req.Prompt)
	}
	if req.ResponseFormat != "" {
		writer.WriteField("response_format", req.ResponseFormat)
	}
	if req.Temperature > 0 {
		writer.WriteField("temperature", fmt.Sprintf("%f", req.Temperature))
	}

	writer.Close()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var transcriptionResp llm.AudioTranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcriptionResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return &transcriptionResp, nil
}

// =============================================================================
// * 嵌入物
// =============================================================================

// CreateEmbedding 为给定输入创建嵌入.
func (p *OpenAIProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/embeddings", strings.TrimRight(p.cfg.BaseURL, "/"))

	payload, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var embeddingResp llm.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return &embeddingResp, nil
}

// =============================================================================
// {\fn黑体\fs22\bord1\shad0\3aHBE\4aH00\fscx67\fscy66\2cHFFFFFF\3cH808080}好图宁 {\fn黑体\fs22\bord1\shad0\3aHBE\4aH00\fscx67\fscy66\2cHFFFFFF\3cH808080}好图宁
// =============================================================================

// 创建 FineTuningJob 创建微调任务.
func (p *OpenAIProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1/fine_tuning/jobs", strings.TrimRight(p.cfg.BaseURL, "/"))

	payload, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var job llm.FineTuningJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return &job, nil
}

// ListFineTuningJobs列出微调工作.
func (p *OpenAIProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1/fine_tuning/jobs", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var jobsResp struct {
		Data []llm.FineTuningJob `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jobsResp); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return jobsResp.Data, nil
}

// Get FineTuningJob通过ID检索微调工作.
func (p *OpenAIProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1/fine_tuning/jobs/%s", strings.TrimRight(p.cfg.BaseURL, "/"), jobID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return nil, providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var job llm.FineTuningJob
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}

	return &job, nil
}

// 取消FineTuningJob取消微调任务.
func (p *OpenAIProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	endpoint := fmt.Sprintf("%s/v1/fine_tuning/jobs/%s/cancel", strings.TrimRight(p.cfg.BaseURL, "/"), jobID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.cfg.APIKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providers.ReadErrorMessage(resp.Body)
		return providers.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	return nil
}

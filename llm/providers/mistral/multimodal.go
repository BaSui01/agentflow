package mistral

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
)

// GenerateImage Mistral 不支持图像生成.
func (p *MistralProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "image generation")
}

// GenerateVideo Mistral 不支持视频生成.
func (p *MistralProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "video generation")
}

// GenerateAudio Mistral 不支持音频生成.
func (p *MistralProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "audio generation")
}

// TranscribeAudio 使用 Voxtral 进行音频转录.
func (p *MistralProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	if req.Model == "" {
		req.Model = "voxtral-mini-transcribe-2-2602"
	}

	endpoint := fmt.Sprintf("%s/v1/audio/transcriptions", strings.TrimRight(p.Cfg.BaseURL, "/"))

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(req.File); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}
	if err := writer.WriteField("model", req.Model); err != nil {
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}
	if req.Language != "" {
		if err := writer.WriteField("language", req.Language); err != nil {
			return nil, fmt.Errorf("failed to write language field: %w", err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var transcriptionResp llm.AudioTranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&transcriptionResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	return &transcriptionResp, nil
}

// CreateEmbedding 使用 Mistral 创建嵌入.
func (p *MistralProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/embeddings", strings.TrimRight(p.Cfg.BaseURL, "/"))

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.ApplyHeaders(httpReq, p.ResolveAPIKey(ctx))
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var embeddingResp llm.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	return &embeddingResp, nil
}

// CreateFineTuningJob Mistral 不支持微调.
func (p *MistralProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// ListFineTuningJobs Mistral 不支持微调.
func (p *MistralProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// GetFineTuningJob Mistral 不支持微调.
func (p *MistralProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	return nil, providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

// CancelFineTuningJob Mistral 不支持微调.
func (p *MistralProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	return providerbase.NotSupportedError(p.Name(), "fine-tuning")
}

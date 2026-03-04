package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	providerbase "github.com/BaSui01/agentflow/llm/providers/base"

	"github.com/BaSui01/agentflow/types"

	"github.com/BaSui01/agentflow/llm"
)

// =============================================================================
// 图像生成
// =============================================================================

// GenerateImage generates images using Gemini model-aware routing.
// imagen-* -> :predict (instances/parameters)
// gemini-*-image-* -> :generateContent (contents/generationConfig)
// others -> OpenAI-compatible predict passthrough
func (p *GeminiProvider) GenerateImage(ctx context.Context, req *llm.ImageGenerationRequest) (*llm.ImageGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "imagen-4.0-generate-001"
	}

	switch imageModelFamily(model) {
	case "imagen":
		endpoint := fmt.Sprintf("/v1beta/models/%s:predict", model)
		body := map[string]any{
			"instances": []map[string]any{
				{"prompt": req.Prompt},
			},
		}
		params := map[string]any{}
		if req.N > 0 {
			params["sampleCount"] = req.N
		}
		if req.NegativePrompt != "" {
			params["negativePrompt"] = req.NegativePrompt
		}
		if ar := chooseAspectRatio(req.Size, ""); ar != "" {
			params["aspectRatio"] = ar
		}
		if req.Quality != "" {
			params["quality"] = req.Quality
		}
		if req.Style != "" {
			params["style"] = req.Style
		}
		if len(params) > 0 {
			body["parameters"] = params
		}

		raw, err := p.postGeminiJSON(ctx, endpoint, body)
		if err != nil {
			return nil, err
		}
		return parseGeminiImageResponse(raw)

	case "gemini-image":
		endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", model)
		body := map[string]any{
			"contents": []map[string]any{
				{
					"role": "user",
					"parts": []map[string]any{
						{"text": req.Prompt},
					},
				},
			},
		}
		generationConfig := map[string]any{
			"responseModalities": []string{"IMAGE"},
		}
		if req.N > 0 {
			generationConfig["candidateCount"] = req.N
		}
		if ar := chooseAspectRatio(req.Size, ""); ar != "" {
			generationConfig["aspectRatio"] = ar
		}
		if req.Quality != "" {
			generationConfig["quality"] = req.Quality
		}
		if req.Style != "" {
			generationConfig["style"] = req.Style
		}
		body["generationConfig"] = generationConfig
		if req.NegativePrompt != "" {
			body["contents"].([]map[string]any)[0]["parts"] = append(
				body["contents"].([]map[string]any)[0]["parts"].([]map[string]any),
				map[string]any{"text": "Avoid: " + req.NegativePrompt},
			)
		}

		raw, err := p.postGeminiJSON(ctx, endpoint, body)
		if err != nil {
			return nil, err
		}
		return parseGeminiImageResponse(raw)

	default:
		endpoint := fmt.Sprintf("/v1beta/models/%s:predict", model)
		return providerbase.GenerateImageOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
	}
}

// =============================================================================
// 视频生成
// =============================================================================

// GenerateVideo uses model-aware routing for Veo models.
// veo-* -> :predictLongRunning (instances/parameters)
// others -> OpenAI-compatible predictLongRunning passthrough
func (p *GeminiProvider) GenerateVideo(ctx context.Context, req *llm.VideoGenerationRequest) (*llm.VideoGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "veo-3.1-generate-preview"
	}

	if videoModelFamily(model) == "veo" {
		endpoint := fmt.Sprintf("/v1beta/models/%s:predictLongRunning", model)
		body := map[string]any{
			"instances": []map[string]any{
				{"prompt": req.Prompt},
			},
		}
		params := map[string]any{}
		if req.Duration > 0 {
			params["durationSeconds"] = req.Duration
		}
		if req.FPS > 0 {
			params["fps"] = req.FPS
		}
		if req.Resolution != "" {
			params["resolution"] = req.Resolution
		}
		if ar := chooseAspectRatio("", req.AspectRatio); ar != "" {
			params["aspectRatio"] = ar
		} else if ar := chooseAspectRatio(req.Resolution, ""); ar != "" {
			params["aspectRatio"] = ar
		}
		if req.Style != "" {
			params["style"] = req.Style
		}
		if len(params) > 0 {
			body["parameters"] = params
		}

		raw, err := p.postGeminiJSON(ctx, endpoint, body)
		if err != nil {
			return nil, err
		}
		return parseGeminiVideoResponse(raw)
	}

	endpoint := fmt.Sprintf("/v1beta/models/%s:predictLongRunning", model)
	return providerbase.GenerateVideoOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
}

func imageModelFamily(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "imagen-"):
		return "imagen"
	case strings.HasPrefix(m, "gemini-") && strings.Contains(m, "image"):
		return "gemini-image"
	default:
		return "other"
	}
}

func videoModelFamily(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "veo-"):
		return "veo"
	default:
		return "other"
	}
}

func chooseAspectRatio(sizeOrResolution, aspectRatio string) string {
	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio != "" {
		return aspectRatio
	}

	s := strings.TrimSpace(sizeOrResolution)
	if s == "" {
		return ""
	}
	sep := "x"
	if strings.Contains(s, ":") {
		return s
	}
	if !strings.Contains(s, sep) {
		return ""
	}
	parts := strings.Split(s, sep)
	if len(parts) != 2 {
		return ""
	}
	w, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || w <= 0 || h <= 0 {
		return ""
	}
	g := gcd(w, h)
	return fmt.Sprintf("%d:%d", w/g, h/g)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

func (p *GeminiProvider) postGeminiJSON(ctx context.Context, endpoint string, body any) ([]byte, error) {
	fullURL := fmt.Sprintf("%s%s", strings.TrimRight(p.cfg.BaseURL, "/"), endpoint)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			Cause:      err,
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrUpstreamError,
			Message:    err.Error(),
			Cause:      err,
			HTTPStatus: http.StatusBadGateway,
			Retryable:  true,
			Provider:   p.Name(),
		}
	}
	return data, nil
}

func parseGeminiImageResponse(raw []byte) (*llm.ImageGenerationResponse, error) {
	var openAIResp llm.ImageGenerationResponse
	if err := json.Unmarshal(raw, &openAIResp); err == nil && len(openAIResp.Data) > 0 {
		if openAIResp.Created == 0 {
			openAIResp.Created = time.Now().Unix()
		}
		return &openAIResp, nil
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("failed to decode gemini image response: %w", err)
	}

	var images []llm.Image
	appendImage := func(img llm.Image) {
		if img.URL == "" && img.B64JSON == "" {
			return
		}
		images = append(images, img)
	}

	if preds, ok := generic["predictions"].([]any); ok {
		for _, item := range preds {
			if m, ok := item.(map[string]any); ok {
				appendImage(extractImageFromMap(m))
			}
		}
	}

	if cands, ok := generic["candidates"].([]any); ok {
		for _, c := range cands {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			content, ok := cm["content"].(map[string]any)
			if !ok {
				continue
			}
			parts, ok := content["parts"].([]any)
			if !ok {
				continue
			}
			for _, p := range parts {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
				}
				appendImage(extractImageFromMap(pm))
			}
		}
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("gemini image response does not contain image data")
	}

	return &llm.ImageGenerationResponse{
		Created: time.Now().Unix(),
		Data:    images,
	}, nil
}

func parseGeminiVideoResponse(raw []byte) (*llm.VideoGenerationResponse, error) {
	var openAIResp llm.VideoGenerationResponse
	if err := json.Unmarshal(raw, &openAIResp); err == nil && (openAIResp.ID != "" || len(openAIResp.Data) > 0) {
		if openAIResp.Created == 0 {
			openAIResp.Created = time.Now().Unix()
		}
		return &openAIResp, nil
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("failed to decode gemini video response: %w", err)
	}

	resp := &llm.VideoGenerationResponse{
		Created: time.Now().Unix(),
	}

	if id, _ := generic["id"].(string); id != "" {
		resp.ID = id
	}
	if name, _ := generic["name"].(string); name != "" {
		resp.ID = name
	}

	collectVideos := func(items []any) {
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			v := llm.Video{}
			if s, _ := m["url"].(string); s != "" {
				v.URL = s
			}
			if s, _ := m["uri"].(string); s != "" && v.URL == "" {
				v.URL = s
			}
			if s, _ := m["videoUri"].(string); s != "" && v.URL == "" {
				v.URL = s
			}
			if s, _ := m["b64_json"].(string); s != "" {
				v.B64JSON = s
			}
			if s, _ := m["bytesBase64Encoded"].(string); s != "" && v.B64JSON == "" {
				v.B64JSON = s
			}
			if v.URL != "" || v.B64JSON != "" {
				resp.Data = append(resp.Data, v)
			}
		}
	}

	if videos, ok := generic["videos"].([]any); ok {
		collectVideos(videos)
	}
	if result, ok := generic["response"].(map[string]any); ok {
		if videos, ok := result["videos"].([]any); ok {
			collectVideos(videos)
		}
		if generated, ok := result["generatedVideos"].([]any); ok {
			collectVideos(generated)
		}
	}

	if resp.ID == "" && len(resp.Data) > 0 {
		resp.ID = "gemini-video-response"
	}
	if resp.ID == "" && len(resp.Data) == 0 {
		return nil, fmt.Errorf("gemini video response does not contain operation id or videos")
	}
	return resp, nil
}

func extractImageFromMap(m map[string]any) llm.Image {
	img := llm.Image{}

	if s, _ := m["url"].(string); s != "" {
		img.URL = s
	}
	if s, _ := m["b64_json"].(string); s != "" {
		img.B64JSON = s
	}
	if s, _ := m["bytesBase64Encoded"].(string); s != "" && img.B64JSON == "" {
		img.B64JSON = s
	}

	for _, key := range []string{"image", "inlineData", "inline_data"} {
		nested, ok := m[key].(map[string]any)
		if !ok {
			continue
		}
		if s, _ := nested["url"].(string); s != "" && img.URL == "" {
			img.URL = s
		}
		if s, _ := nested["uri"].(string); s != "" && img.URL == "" {
			img.URL = s
		}
		if s, _ := nested["imageUri"].(string); s != "" && img.URL == "" {
			img.URL = s
		}
		if s, _ := nested["b64_json"].(string); s != "" && img.B64JSON == "" {
			img.B64JSON = s
		}
		if s, _ := nested["bytesBase64Encoded"].(string); s != "" && img.B64JSON == "" {
			img.B64JSON = s
		}
		if s, _ := nested["imageBytes"].(string); s != "" && img.B64JSON == "" {
			img.B64JSON = s
		}
		if s, _ := nested["data"].(string); s != "" && img.B64JSON == "" {
			img.B64JSON = s
		}
	}

	return img
}

// =============================================================================
// 音频生成和转录
// =============================================================================

// GenerateAudio 使用 Gemini TTS 生成音频.
// Endpoint: POST /v1beta/models/{model}:generateContent
// Models: gemini-2.5-flash-preview-tts, gemini-2.5-pro-preview-tts
// Supports 30+ voices including Kore, Charon, Fenrir, Aoede, Puck, etc.
func (p *GeminiProvider) GenerateAudio(ctx context.Context, req *llm.AudioGenerationRequest) (*llm.AudioGenerationResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash-preview-tts"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", model)
	return providerbase.GenerateAudioOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
}

// TranscribeAudio 使用 Gemini 进行语音转文本.
// Endpoint: POST /v1beta/models/{model}:generateContent
// Models: gemini-2.5-flash (with audio inline data)
func (p *GeminiProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	// Gemini STT 通过 generateContent + inline audio data 实现
	import64 := base64.StdEncoding.EncodeToString(req.File)
	mimeType := "audio/mp3"
	if req.ResponseFormat != "" {
		mimeType = "audio/" + req.ResponseFormat
	}

	geminiReq := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{"inline_data": map[string]any{"mime_type": mimeType, "data": import64}},
					{"text": "Transcribe this audio to text. Return only the transcription."},
				},
			},
		},
	}
	if req.Language != "" {
		geminiReq["contents"].([]map[string]any)[0]["parts"].([]map[string]any)[1]["text"] = fmt.Sprintf("Transcribe this audio to text in %s. Return only the transcription.", req.Language)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(p.cfg.BaseURL, "/"), model)
	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	text := ""
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		text = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	return &llm.AudioTranscriptionResponse{
		Text:     text,
		Language: req.Language,
	}, nil
}

// =============================================================================
// 嵌入
// =============================================================================

// CreateEmbedding creates embeddings using Gemini.
// Endpoint: POST /v1beta/models/{model}:embedContent
// Models: gemini-embedding-001 (latest, MRL, 128-3072 dims), text-embedding-004 (legacy)
func (p *GeminiProvider) CreateEmbedding(ctx context.Context, req *llm.EmbeddingRequest) (*llm.EmbeddingResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-embedding-001"
	}
	endpoint := fmt.Sprintf("/v1beta/models/%s:embedContent", model)
	return providerbase.CreateEmbeddingOpenAICompat(ctx, p.client, p.cfg.BaseURL, p.resolveAPIKey(ctx), p.Name(), endpoint, req, p.buildHeaders)
}

// =============================================================================
// 微调
// =============================================================================

// CreateFineTuningJob 使用 Gemini 创建微调任务.
// Endpoint: POST /v1beta/tunedModels
func (p *GeminiProvider) CreateFineTuningJob(ctx context.Context, req *llm.FineTuningJobRequest) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1beta/tunedModels", strings.TrimRight(p.cfg.BaseURL, "/"))

	geminiReq := map[string]any{
		"baseModel":        req.Model,
		"displayName":      req.Suffix,
		"tuningTask":       map[string]any{"hyperparameters": req.Hyperparameters},
		"trainingDatasets": req.TrainingFile,
	}

	payload, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var geminiResp struct {
		Name     string `json:"name"`
		Metadata struct {
			TotalSteps int    `json:"totalSteps"`
			TunedModel string `json:"tunedModel"`
		} `json:"metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	return &llm.FineTuningJob{
		ID:     geminiResp.Name,
		Model:  req.Model,
		Status: "queued",
	}, nil
}

// ListFineTuningJobs 列出 Gemini 微调任务.
// Endpoint: GET /v1beta/tunedModels
func (p *GeminiProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1beta/tunedModels", strings.TrimRight(p.cfg.BaseURL, "/"))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var listResp struct {
		TunedModels []struct {
			Name        string `json:"name"`
			BaseModel   string `json:"baseModel"`
			DisplayName string `json:"displayName"`
			State       string `json:"state"`
		} `json:"tunedModels"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	jobs := make([]llm.FineTuningJob, len(listResp.TunedModels))
	for i, m := range listResp.TunedModels {
		jobs[i] = llm.FineTuningJob{
			ID:     m.Name,
			Model:  m.BaseModel,
			Status: strings.ToLower(m.State),
		}
	}
	return jobs, nil
}

// GetFineTuningJob 获取 Gemini 微调任务.
// Endpoint: GET /v1beta/{name}
func (p *GeminiProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	endpoint := fmt.Sprintf("%s/v1beta/%s", strings.TrimRight(p.cfg.BaseURL, "/"), jobID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return nil, providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}

	var tunedModel struct {
		Name      string `json:"name"`
		BaseModel string `json:"baseModel"`
		State     string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tunedModel); err != nil {
		return nil, &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Provider: p.Name()}
	}

	return &llm.FineTuningJob{
		ID:     tunedModel.Name,
		Model:  tunedModel.BaseModel,
		Status: strings.ToLower(tunedModel.State),
	}, nil
}

// CancelFineTuningJob 取消 Gemini 微调任务.
// Endpoint: DELETE /v1beta/{name}
func (p *GeminiProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	endpoint := fmt.Sprintf("%s/v1beta/%s", strings.TrimRight(p.cfg.BaseURL, "/"), jobID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	p.buildHeaders(httpReq, p.resolveAPIKey(ctx))

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return &types.Error{Code: llm.ErrUpstreamError, Message: err.Error(), Cause: err, HTTPStatus: http.StatusBadGateway, Retryable: true, Provider: p.Name()}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := providerbase.ReadErrorMessage(resp.Body)
		return providerbase.MapHTTPError(resp.StatusCode, msg, p.Name())
	}
	return nil
}

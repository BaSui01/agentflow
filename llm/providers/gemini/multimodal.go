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
	"google.golang.org/genai"

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

	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	switch imageModelFamily(model) {
	case "imagen":
		cfg := &genai.GenerateImagesConfig{
			NegativePrompt: req.NegativePrompt,
		}
		if req.N > 0 {
			cfg.NumberOfImages = int32(req.N)
		}
		if ar := chooseAspectRatio(req.Size, ""); ar != "" {
			cfg.AspectRatio = ar
		}
		resp, err := client.Models.GenerateImages(ctx, model, req.Prompt, cfg)
		if err != nil {
			return nil, p.mapSDKError(err)
		}
		return imageGenerationResponseFromGenAI(resp), nil

	case "gemini-image":
		prompt := req.Prompt
		if req.Style != "" {
			prompt += "\nStyle: " + req.Style
		}
		if req.Quality != "" {
			prompt += "\nQuality: " + req.Quality
		}
		if req.NegativePrompt != "" {
			prompt += "\nAvoid: " + req.NegativePrompt
		}

		cfg := &genai.GenerateContentConfig{
			ResponseModalities: []string{"IMAGE"},
			ImageConfig:        &genai.ImageConfig{},
		}
		if req.N > 0 {
			cfg.CandidateCount = int32(req.N)
		}
		if ar := chooseAspectRatio(req.Size, ""); ar != "" {
			cfg.ImageConfig.AspectRatio = ar
		}
		resp, err := client.Models.GenerateContent(ctx, model, genai.Text(prompt), cfg)
		if err != nil {
			return nil, p.mapSDKError(err)
		}
		return imageGenerationResponseFromGenAIContent(resp), nil

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
		client, err := p.sdkClient(ctx)
		if err != nil {
			return nil, p.mapSDKError(err)
		}

		cfg := &genai.GenerateVideosConfig{}
		if req.Duration > 0 {
			v := int32(req.Duration)
			cfg.DurationSeconds = &v
		}
		if req.FPS > 0 {
			v := int32(req.FPS)
			cfg.FPS = &v
		}
		if req.Resolution != "" {
			cfg.Resolution = req.Resolution
		}
		if ar := chooseAspectRatio("", req.AspectRatio); ar != "" {
			cfg.AspectRatio = ar
		} else if ar := chooseAspectRatio(req.Resolution, ""); ar != "" {
			cfg.AspectRatio = ar
		}

		op, err := client.Models.GenerateVideos(ctx, model, req.Prompt, nil, cfg)
		if err != nil {
			return nil, p.mapSDKError(err)
		}
		return videoGenerationResponseFromGenAI(op), nil
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

func imageGenerationResponseFromGenAI(resp *genai.GenerateImagesResponse) *llm.ImageGenerationResponse {
	out := &llm.ImageGenerationResponse{
		Created: time.Now().Unix(),
	}
	if resp == nil {
		return out
	}
	for _, item := range resp.GeneratedImages {
		if item == nil || item.Image == nil {
			continue
		}
		image := llm.Image{
			URL:           item.Image.GCSURI,
			RevisedPrompt: item.EnhancedPrompt,
		}
		if len(item.Image.ImageBytes) > 0 {
			image.B64JSON = base64.StdEncoding.EncodeToString(item.Image.ImageBytes)
		}
		if image.URL != "" || image.B64JSON != "" {
			out.Data = append(out.Data, image)
		}
	}
	return out
}

func imageGenerationResponseFromGenAIContent(resp *genai.GenerateContentResponse) *llm.ImageGenerationResponse {
	out := &llm.ImageGenerationResponse{
		Created: time.Now().Unix(),
	}
	if resp == nil {
		return out
	}
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part == nil || part.InlineData == nil || len(part.InlineData.Data) == 0 {
				continue
			}
			out.Data = append(out.Data, llm.Image{
				B64JSON: base64.StdEncoding.EncodeToString(part.InlineData.Data),
			})
		}
	}
	return out
}

func videoGenerationResponseFromGenAI(op *genai.GenerateVideosOperation) *llm.VideoGenerationResponse {
	out := &llm.VideoGenerationResponse{
		Created: time.Now().Unix(),
	}
	if op == nil {
		return out
	}
	out.ID = op.Name
	if op.Response == nil {
		return out
	}
	for _, item := range op.Response.GeneratedVideos {
		if item == nil || item.Video == nil {
			continue
		}
		video := llm.Video{URL: item.Video.URI}
		if len(item.Video.VideoBytes) > 0 {
			video.B64JSON = base64.StdEncoding.EncodeToString(item.Video.VideoBytes)
		}
		if video.URL != "" || video.B64JSON != "" {
			out.Data = append(out.Data, video)
		}
	}
	return out
}

func extractAudioBytesFromGenAI(resp *genai.GenerateContentResponse) []byte {
	if resp == nil {
		return nil
	}
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part != nil && part.InlineData != nil && len(part.InlineData.Data) > 0 {
				return part.InlineData.Data
			}
		}
	}
	return nil
}

func llmEmbeddingResponseFromGenAI(resp *genai.EmbedContentResponse, model string) *llm.EmbeddingResponse {
	out := &llm.EmbeddingResponse{
		Object: "list",
		Model:  model,
	}
	if resp == nil {
		return out
	}
	out.Data = make([]llm.Embedding, 0, len(resp.Embeddings))
	for i, item := range resp.Embeddings {
		if item == nil {
			continue
		}
		vector := make([]float64, 0, len(item.Values))
		for _, v := range item.Values {
			vector = append(vector, float64(v))
		}
		out.Data = append(out.Data, llm.Embedding{
			Object:    "embedding",
			Index:     i,
			Embedding: vector,
		})
	}
	return out
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

	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	cfg := &genai.GenerateContentConfig{
		ResponseModalities: []string{"AUDIO"},
	}
	if req.Voice != "" {
		cfg.SpeechConfig = &genai.SpeechConfig{
			VoiceConfig: &genai.VoiceConfig{
				PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{VoiceName: req.Voice},
			},
		}
	}

	resp, err := client.Models.GenerateContent(ctx, model, genai.Text(req.Input), cfg)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	audio := extractAudioBytesFromGenAI(resp)
	if len(audio) == 0 {
		return nil, fmt.Errorf("gemini audio response does not contain audio data")
	}
	return &llm.AudioGenerationResponse{Audio: audio}, nil
}

// TranscribeAudio 使用 Gemini 进行语音转文本.
// Endpoint: POST /v1beta/models/{model}:generateContent
// Models: gemini-2.5-flash (with audio inline data)
func (p *GeminiProvider) TranscribeAudio(ctx context.Context, req *llm.AudioTranscriptionRequest) (*llm.AudioTranscriptionResponse, error) {
	model := req.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	mimeType := "audio/mpeg"
	if req.ResponseFormat != "" {
		mimeType = "audio/" + req.ResponseFormat
	}
	prompt := "Transcribe this audio to text. Return only the transcription."
	if req.Language != "" {
		prompt = fmt.Sprintf("Transcribe this audio to text in %s. Return only the transcription.", req.Language)
	}
	if strings.TrimSpace(req.Prompt) != "" {
		prompt += "\nAdditional instructions: " + strings.TrimSpace(req.Prompt)
	}

	contents := []*genai.Content{
		genai.NewContentFromParts([]*genai.Part{
			genai.NewPartFromBytes(req.File, mimeType),
			genai.NewPartFromText(prompt),
		}, genai.RoleUser),
	}

	resp, err := client.Models.GenerateContent(ctx, model, contents, &genai.GenerateContentConfig{})
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	text := strings.TrimSpace(resp.Text())

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

	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	contents := make([]*genai.Content, 0, len(req.Input))
	for _, input := range req.Input {
		contents = append(contents, genai.NewContentFromText(input, genai.RoleUser))
	}
	cfg := &genai.EmbedContentConfig{}
	if req.Dimensions > 0 {
		dims := int32(req.Dimensions)
		cfg.OutputDimensionality = &dims
	}

	resp, err := client.Models.EmbedContent(ctx, model, contents, cfg)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	return llmEmbeddingResponseFromGenAI(resp, model), nil
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

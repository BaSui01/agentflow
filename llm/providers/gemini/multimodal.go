package gemini

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/BaSui01/agentflow/llm"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
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
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	dataset := &genai.TuningDataset{}
	if trimmed := strings.TrimSpace(req.TrainingFile); trimmed != "" {
		dataset.GCSURI = trimmed
	}
	config := buildGeminiTuningConfig(req)
	job, err := client.Tunings.Tune(ctx, req.Model, dataset, config)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	return fineTuningJobFromGenAI(job), nil
}

// ListFineTuningJobs 列出 Gemini 微调任务.
// Endpoint: GET /v1beta/tunedModels
func (p *GeminiProvider) ListFineTuningJobs(ctx context.Context) ([]llm.FineTuningJob, error) {
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	jobs := make([]llm.FineTuningJob, 0)
	for job, iterErr := range client.Tunings.All(ctx) {
		if iterErr != nil {
			return nil, p.mapSDKError(iterErr)
		}
		if job == nil {
			continue
		}
		jobs = append(jobs, *fineTuningJobFromGenAI(job))
	}
	return jobs, nil
}

// GetFineTuningJob 获取 Gemini 微调任务.
// Endpoint: GET /v1beta/{name}
func (p *GeminiProvider) GetFineTuningJob(ctx context.Context, jobID string) (*llm.FineTuningJob, error) {
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	job, err := client.Tunings.Get(ctx, jobID, nil)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	return fineTuningJobFromGenAI(job), nil
}

// CancelFineTuningJob 取消 Gemini 微调任务.
// Endpoint: DELETE /v1beta/{name}
func (p *GeminiProvider) CancelFineTuningJob(ctx context.Context, jobID string) error {
	client, err := p.sdkClient(ctx)
	if err != nil {
		return p.mapSDKError(err)
	}
	_, err = client.Tunings.Cancel(ctx, jobID, nil)
	if err != nil {
		return p.mapSDKError(err)
	}
	return nil
}

func buildGeminiTuningConfig(req *llm.FineTuningJobRequest) *genai.CreateTuningJobConfig {
	if req == nil {
		return nil
	}
	cfg := &genai.CreateTuningJobConfig{
		TunedModelDisplayName: strings.TrimSpace(req.Suffix),
	}
	if req.ValidationFile != "" {
		cfg.ValidationDataset = &genai.TuningValidationDataset{GCSURI: strings.TrimSpace(req.ValidationFile)}
	}
	if hp := req.Hyperparameters; len(hp) > 0 {
		if epochs, ok := hyperparamInt32(hp["epoch_count"], hp["epochs"], hp["n_epochs"]); ok {
			cfg.EpochCount = &epochs
		}
		if batchSize, ok := hyperparamInt32(hp["batch_size"]); ok {
			cfg.BatchSize = &batchSize
		}
		if lrMult, ok := hyperparamFloat32(hp["learning_rate_multiplier"]); ok {
			cfg.LearningRateMultiplier = &lrMult
		}
		if lr, ok := hyperparamFloat32(hp["learning_rate"]); ok {
			cfg.LearningRate = &lr
		}
	}
	if cfg.ValidationDataset == nil && cfg.TunedModelDisplayName == "" && cfg.EpochCount == nil && cfg.BatchSize == nil && cfg.LearningRateMultiplier == nil && cfg.LearningRate == nil {
		return nil
	}
	return cfg
}

func fineTuningJobFromGenAI(job *genai.TuningJob) *llm.FineTuningJob {
	if job == nil {
		return &llm.FineTuningJob{}
	}
	out := &llm.FineTuningJob{
		ID:         strings.TrimSpace(job.Name),
		Model:      strings.TrimSpace(job.BaseModel),
		Status:     strings.ToLower(strings.TrimPrefix(string(job.State), "JOB_STATE_")),
		CreatedAt:  job.CreateTime.Unix(),
		FinishedAt: job.EndTime.Unix(),
	}
	if job.TunedModel != nil {
		out.FineTunedModel = strings.TrimSpace(job.TunedModel.Model)
	}
	if out.FineTunedModel == "" && job.TunedModelDisplayName != "" {
		out.FineTunedModel = strings.TrimSpace(job.TunedModelDisplayName)
	}
	if job.Error != nil {
		out.Error = &llm.FineTuningError{
			Code:    fmt.Sprintf("%d", job.Error.Code),
			Message: strings.TrimSpace(job.Error.Message),
		}
	}
	return out
}

func hyperparamInt32(values ...any) (int32, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case int:
			return int32(typed), true
		case int32:
			return typed, true
		case int64:
			return int32(typed), true
		case float64:
			return int32(typed), true
		}
	}
	return 0, false
}

func hyperparamFloat32(values ...any) (float32, bool) {
	for _, value := range values {
		switch typed := value.(type) {
		case float32:
			return typed, true
		case float64:
			return float32(typed), true
		case int:
			return float32(typed), true
		}
	}
	return 0, false
}

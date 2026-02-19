package llm

import "context"

// =============================================================================
// 🎨 MultiModal Provider Interfaces
// =============================================================================

// MultiModalProvider extends Provider with multimodal capabilities.
type MultiModalProvider interface {
	Provider

	// GenerateImage generates an image from a text prompt.
	// Returns nil if the provider doesn't support image generation.
	GenerateImage(ctx context.Context, req *ImageGenerationRequest) (*ImageGenerationResponse, error)

	// GenerateVideo generates a video from a text prompt.
	// Returns nil if the provider doesn't support video generation.
	GenerateVideo(ctx context.Context, req *VideoGenerationRequest) (*VideoGenerationResponse, error)

	// GenerateAudio generates audio/speech from text.
	// Returns nil if the provider doesn't support audio generation.
	GenerateAudio(ctx context.Context, req *AudioGenerationRequest) (*AudioGenerationResponse, error)

	// TranscribeAudio transcribes audio to text.
	// Returns nil if the provider doesn't support audio transcription.
	TranscribeAudio(ctx context.Context, req *AudioTranscriptionRequest) (*AudioTranscriptionResponse, error)
}

// EmbeddingProvider extends Provider with embedding capabilities.
type EmbeddingProvider interface {
	Provider

	// CreateEmbedding creates embeddings for the given input.
	CreateEmbedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)
}

// FineTuningProvider extends Provider with fine-tuning capabilities.
type FineTuningProvider interface {
	Provider

	// CreateFineTuningJob creates a fine-tuning job.
	CreateFineTuningJob(ctx context.Context, req *FineTuningJobRequest) (*FineTuningJob, error)

	// ListFineTuningJobs lists fine-tuning jobs.
	ListFineTuningJobs(ctx context.Context) ([]FineTuningJob, error)

	// GetFineTuningJob retrieves a fine-tuning job by ID.
	GetFineTuningJob(ctx context.Context, jobID string) (*FineTuningJob, error)

	// CancelFineTuningJob cancels a fine-tuning job.
	CancelFineTuningJob(ctx context.Context, jobID string) error
}

// =============================================================================
// 🖼️ Image Generation Types
// =============================================================================

// ImageGenerationRequest represents an image generation request.
type ImageGenerationRequest struct {
	Model          string  `json:"model"`                     // 模型名称
	Prompt         string  `json:"prompt"`                    // 文本提示
	NegativePrompt string  `json:"negative_prompt,omitempty"` // 负面提示
	N              int     `json:"n,omitempty"`               // 生成图片数量
	Size           string  `json:"size,omitempty"`            // 图片尺寸（如 "1024x1024"）
	Quality        string  `json:"quality,omitempty"`         // 图片质量（standard, hd）
	Style          string  `json:"style,omitempty"`           // 图片风格
	ResponseFormat string  `json:"response_format,omitempty"` // 响应格式（url, b64_json）
	User           string  `json:"user,omitempty"`            // 用户标识
}

// ImageGenerationResponse represents an image generation response.
type ImageGenerationResponse struct {
	Created int64   `json:"created"`
	Data    []Image `json:"data"`
}

// Image represents a generated image.
type Image struct {
	URL           string `json:"url,omitempty"`            // 图片 URL
	B64JSON       string `json:"b64_json,omitempty"`       // Base64 编码的图片
	RevisedPrompt string `json:"revised_prompt,omitempty"` // 修订后的提示
}

// =============================================================================
// 🎬 Video Generation Types
// =============================================================================

// VideoGenerationRequest represents a video generation request.
type VideoGenerationRequest struct {
	Model          string  `json:"model"`                     // 模型名称
	Prompt         string  `json:"prompt"`                    // 文本提示
	Duration       int     `json:"duration,omitempty"`        // 视频时长（秒）
	FPS            int     `json:"fps,omitempty"`             // 帧率
	Resolution     string  `json:"resolution,omitempty"`      // 分辨率（如 "1920x1080"）
	AspectRatio    string  `json:"aspect_ratio,omitempty"`    // 宽高比（如 "16:9"）
	Style          string  `json:"style,omitempty"`           // 视频风格
	ResponseFormat string  `json:"response_format,omitempty"` // 响应格式（url, b64_json）
}

// VideoGenerationResponse represents a video generation response.
type VideoGenerationResponse struct {
	ID      string  `json:"id"`
	Created int64   `json:"created"`
	Data    []Video `json:"data"`
}

// Video represents a generated video.
type Video struct {
	URL     string `json:"url,omitempty"`      // 视频 URL
	B64JSON string `json:"b64_json,omitempty"` // Base64 编码的视频
}

// =============================================================================
// 🎵 Audio Generation & Transcription Types
// =============================================================================

// AudioGenerationRequest represents an audio/speech generation request.
type AudioGenerationRequest struct {
	Model          string  `json:"model"`                     // 模型名称
	Input          string  `json:"input"`                     // 输入文本
	Voice          string  `json:"voice,omitempty"`           // 语音类型
	Speed          float32 `json:"speed,omitempty"`           // 语速（0.25 - 4.0）
	ResponseFormat string  `json:"response_format,omitempty"` // 响应格式（mp3, opus, aac, flac）
}

// AudioGenerationResponse represents an audio generation response.
type AudioGenerationResponse struct {
	Audio []byte `json:"audio"` // 音频数据
}

// AudioTranscriptionRequest represents an audio transcription request.
type AudioTranscriptionRequest struct {
	Model          string  `json:"model"`                     // 模型名称
	File           []byte  `json:"file"`                      // 音频文件数据
	Language       string  `json:"language,omitempty"`        // 语言代码（如 "en", "zh"）
	Prompt         string  `json:"prompt,omitempty"`          // 可选的提示文本
	ResponseFormat string  `json:"response_format,omitempty"` // 响应格式（json, text, srt, vtt）
	Temperature    float32 `json:"temperature,omitempty"`     // 采样温度
}

// AudioTranscriptionResponse represents an audio transcription response.
type AudioTranscriptionResponse struct {
	Text     string                 `json:"text"`               // 转录文本
	Language string                 `json:"language,omitempty"` // 检测到的语言
	Duration float64                `json:"duration,omitempty"` // 音频时长（秒）
	Segments []TranscriptionSegment `json:"segments,omitempty"` // 分段信息
}

// TranscriptionSegment represents a segment of transcribed audio.
type TranscriptionSegment struct {
	ID               int     `json:"id"`
	Seek             int     `json:"seek"`
	Start            float64 `json:"start"`
	End              float64 `json:"end"`
	Text             string  `json:"text"`
	Tokens           []int   `json:"tokens"`
	Temperature      float32 `json:"temperature"`
	AvgLogprob       float64 `json:"avg_logprob"`
	CompressionRatio float64 `json:"compression_ratio"`
	NoSpeechProb     float64 `json:"no_speech_prob"`
}

// =============================================================================
// 📝 Embedding Types
// =============================================================================

// EmbeddingRequest represents an embedding request.
type EmbeddingRequest struct {
	Model          string   `json:"model"`                     // 模型名称
	Input          []string `json:"input"`                     // 输入文本列表
	EncodingFormat string   `json:"encoding_format,omitempty"` // 编码格式（float, base64）
	Dimensions     int      `json:"dimensions,omitempty"`      // 输出维度
	User           string   `json:"user,omitempty"`            // 用户标识
}

// EmbeddingResponse represents an embedding response.
type EmbeddingResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  ChatUsage   `json:"usage"`
}

// Embedding represents a single embedding.
type Embedding struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// =============================================================================
// 🔄 Fine-Tuning Types
// =============================================================================

// FineTuningJobRequest represents a fine-tuning job creation request.
type FineTuningJobRequest struct {
	Model           string                 `json:"model"`                       // 基础模型
	TrainingFile    string                 `json:"training_file"`               // 训练文件 ID
	ValidationFile  string                 `json:"validation_file,omitempty"`   // 验证文件 ID
	Hyperparameters map[string]interface{} `json:"hyperparameters,omitempty"`   // 超参数
	Suffix          string                 `json:"suffix,omitempty"`            // 模型名称后缀
	IntegrationIDs  []string               `json:"integration_ids,omitempty"`   // 集成 ID
}

// FineTuningJob represents a fine-tuning job.
type FineTuningJob struct {
	ID              string                 `json:"id"`
	Object          string                 `json:"object"`
	Model           string                 `json:"model"`
	CreatedAt       int64                  `json:"created_at"`
	FinishedAt      int64                  `json:"finished_at,omitempty"`
	FineTunedModel  string                 `json:"fine_tuned_model,omitempty"`
	OrganizationID  string                 `json:"organization_id"`
	ResultFiles     []string               `json:"result_files"`
	Status          string                 `json:"status"` // queued, running, succeeded, failed, cancelled
	ValidationFile  string                 `json:"validation_file,omitempty"`
	TrainingFile    string                 `json:"training_file"`
	Hyperparameters map[string]interface{} `json:"hyperparameters"`
	TrainedTokens   int                    `json:"trained_tokens,omitempty"`
	Error           *FineTuningError       `json:"error,omitempty"`
}

// FineTuningError represents a fine-tuning error.
type FineTuningError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

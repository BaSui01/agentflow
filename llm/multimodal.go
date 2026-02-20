package llm

import "context"

// =============================================================================
// 多模态提供者接口
// =============================================================================

// MultiModalProvider 扩展 Provider，具有多模态能力.
type MultiModalProvider interface {
	Provider

	// GenerateImage 从文本提示生成图像.
	// 如果提供者不支持图像生成，则返回 nil。
	GenerateImage(ctx context.Context, req *ImageGenerationRequest) (*ImageGenerationResponse, error)

	// GenerateVideo 从文本提示生成视频.
	// 如果提供者不支持视频生成，则返回 nil。
	GenerateVideo(ctx context.Context, req *VideoGenerationRequest) (*VideoGenerationResponse, error)

	// GenerateAudio 从文本生成音频/语音.
	// 如果提供者不支持音频生成，则返回 nil。
	GenerateAudio(ctx context.Context, req *AudioGenerationRequest) (*AudioGenerationResponse, error)

	// TranscribeAudio 将音频转录为文本.
	// 如果提供者不支持音频转录，则返回 nil。
	TranscribeAudio(ctx context.Context, req *AudioTranscriptionRequest) (*AudioTranscriptionResponse, error)
}

// EmbeddingProvider 扩展 Provider，具有嵌入能力.
type EmbeddingProvider interface {
	Provider

	// CreateEmbedding 为给定输入创建嵌入.
	CreateEmbedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error)
}

// FineTuningProvider 扩展 Provider，具有微调能力.
type FineTuningProvider interface {
	Provider

	// CreateFineTuningJob 创建微调任务.
	CreateFineTuningJob(ctx context.Context, req *FineTuningJobRequest) (*FineTuningJob, error)

	// ListFineTuningJobs 列出微调任务.
	ListFineTuningJobs(ctx context.Context) ([]FineTuningJob, error)

	// GetFineTuningJob 通过 ID 获取微调任务.
	GetFineTuningJob(ctx context.Context, jobID string) (*FineTuningJob, error)

	// CancelFineTuningJob 取消微调任务.
	CancelFineTuningJob(ctx context.Context, jobID string) error
}

// =============================================================================
// 图像生成类型
// =============================================================================

// ImageGenerationRequest 表示图像生成请求.
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

// ImageGenerationResponse 表示图像生成响应.
type ImageGenerationResponse struct {
	Created int64   `json:"created"`
	Data    []Image `json:"data"`
}

// Image 表示生成的图像.
type Image struct {
	URL           string `json:"url,omitempty"`            // 图片 URL
	B64JSON       string `json:"b64_json,omitempty"`       // Base64 编码的图片
	RevisedPrompt string `json:"revised_prompt,omitempty"` // 修订后的提示
}

// =============================================================================
// 视频生成类型
// =============================================================================

// VideoGenerationRequest 表示视频生成请求.
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

// VideoGenerationResponse 表示视频生成响应.
type VideoGenerationResponse struct {
	ID      string  `json:"id"`
	Created int64   `json:"created"`
	Data    []Video `json:"data"`
}

// Video 表示生成的视频.
type Video struct {
	URL     string `json:"url,omitempty"`      // 视频 URL
	B64JSON string `json:"b64_json,omitempty"` // Base64 编码的视频
}

// =============================================================================
// 音频生成与转录类型
// =============================================================================

// AudioGenerationRequest 表示音频/语音生成请求.
type AudioGenerationRequest struct {
	Model          string  `json:"model"`                     // 模型名称
	Input          string  `json:"input"`                     // 输入文本
	Voice          string  `json:"voice,omitempty"`           // 语音类型
	Speed          float32 `json:"speed,omitempty"`           // 语速（0.25 - 4.0）
	ResponseFormat string  `json:"response_format,omitempty"` // 响应格式（mp3, opus, aac, flac）
}

// AudioGenerationResponse 表示音频生成响应.
type AudioGenerationResponse struct {
	Audio []byte `json:"audio"` // 音频数据
}

// AudioTranscriptionRequest 表示音频转录请求.
type AudioTranscriptionRequest struct {
	Model          string  `json:"model"`                     // 模型名称
	File           []byte  `json:"file"`                      // 音频文件数据
	Language       string  `json:"language,omitempty"`        // 语言代码（如 "en", "zh"）
	Prompt         string  `json:"prompt,omitempty"`          // 可选的提示文本
	ResponseFormat string  `json:"response_format,omitempty"` // 响应格式（json, text, srt, vtt）
	Temperature    float32 `json:"temperature,omitempty"`     // 采样温度
}

// AudioTranscriptionResponse 表示音频转录响应.
type AudioTranscriptionResponse struct {
	Text     string                 `json:"text"`               // 转录文本
	Language string                 `json:"language,omitempty"` // 检测到的语言
	Duration float64                `json:"duration,omitempty"` // 音频时长（秒）
	Segments []TranscriptionSegment `json:"segments,omitempty"` // 分段信息
}

// TranscriptionSegment 表示转录音频的一个片段.
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
// 嵌入类型
// =============================================================================

// EmbeddingRequest 表示嵌入请求.
type EmbeddingRequest struct {
	Model          string   `json:"model"`                     // 模型名称
	Input          []string `json:"input"`                     // 输入文本列表
	EncodingFormat string   `json:"encoding_format,omitempty"` // 编码格式（float, base64）
	Dimensions     int      `json:"dimensions,omitempty"`      // 输出维度
	User           string   `json:"user,omitempty"`            // 用户标识
}

// EmbeddingResponse 表示嵌入响应.
type EmbeddingResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  ChatUsage   `json:"usage"`
}

// Embedding 表示单个嵌入向量.
type Embedding struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}

// =============================================================================
// 微调类型
// =============================================================================

// FineTuningJobRequest 表示创建微调任务的请求.
type FineTuningJobRequest struct {
	Model           string                 `json:"model"`                       // 基础模型
	TrainingFile    string                 `json:"training_file"`               // 训练文件 ID
	ValidationFile  string                 `json:"validation_file,omitempty"`   // 验证文件 ID
	Hyperparameters map[string]interface{} `json:"hyperparameters,omitempty"`   // 超参数
	Suffix          string                 `json:"suffix,omitempty"`            // 模型名称后缀
	IntegrationIDs  []string               `json:"integration_ids,omitempty"`   // 集成 ID
}

// FineTuningJob 表示微调任务.
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

// FineTuningError 表示微调错误.
type FineTuningError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

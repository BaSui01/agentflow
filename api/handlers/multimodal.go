package handlers

import (
	"encoding/base64"
	"io"
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/llm/image"
	"github.com/BaSui01/agentflow/llm/moderation"
	"github.com/BaSui01/agentflow/llm/multimodal"
	"github.com/BaSui01/agentflow/llm/music"
	"github.com/BaSui01/agentflow/llm/speech"
	"github.com/BaSui01/agentflow/llm/threed"
	"github.com/BaSui01/agentflow/llm/video"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// MultimodalHandler 处理多模态 API 请求
type MultimodalHandler struct {
	router *multimodal.Router
	logger *zap.Logger
}

// NewMultimodalHandler 创建多模态 handler
func NewMultimodalHandler(router *multimodal.Router, logger *zap.Logger) *MultimodalHandler {
	return &MultimodalHandler{router: router, logger: logger}
}

// HandleListProviders 返回已注册的 provider 列表
func (h *MultimodalHandler) HandleListProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	WriteSuccess(w, h.router.ListProviders())
}

// HandleGenerateImage 处理图像生成请求
func (h *MultimodalHandler) HandleGenerateImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}
	var req struct {
		image.GenerateRequest
		Provider string `json:"provider,omitempty"`
	}
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, err := h.router.GenerateImage(r.Context(), &req.GenerateRequest, req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}

// editImageRequest is a JSON-friendly wrapper for image edit requests.
// image.EditRequest uses io.Reader fields which cannot be decoded from JSON.
type editImageRequest struct {
	ImageData      string            `json:"image_data"`                // Base64 encoded image
	MaskData       string            `json:"mask_data,omitempty"`       // Base64 encoded mask
	Prompt         string            `json:"prompt"`
	Model          string            `json:"model,omitempty"`
	N              int               `json:"n,omitempty"`
	Size           string            `json:"size,omitempty"`
	ResponseFormat string            `json:"response_format,omitempty"`
	Provider       string            `json:"provider,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// HandleEditImage 处理图像编辑请求
func (h *MultimodalHandler) HandleEditImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req editImageRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}
	if req.ImageData == "" {
		WriteError(w, types.NewInvalidRequestError("image_data is required"), h.logger)
		return
	}

	imgBytes, err := base64.StdEncoding.DecodeString(req.ImageData)
	if err != nil {
		WriteError(w, types.NewInvalidRequestError("invalid base64 image_data"), h.logger)
		return
	}

	editReq := &image.EditRequest{
		Image:          strings.NewReader(string(imgBytes)),
		Prompt:         req.Prompt,
		Model:          req.Model,
		N:              req.N,
		Size:           req.Size,
		ResponseFormat: req.ResponseFormat,
		Metadata:       req.Metadata,
	}
	if req.MaskData != "" {
		maskBytes, err := base64.StdEncoding.DecodeString(req.MaskData)
		if err != nil {
			WriteError(w, types.NewInvalidRequestError("invalid base64 mask_data"), h.logger)
			return
		}
		editReq.Mask = strings.NewReader(string(maskBytes))
	}

	p, err := h.router.Image(req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	resp, err := p.Edit(r.Context(), editReq)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}

// HandleAnalyzeVideo 处理视频分析请求
func (h *MultimodalHandler) HandleAnalyzeVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req struct {
		video.AnalyzeRequest
		Provider string `json:"provider,omitempty"`
	}
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	p, err := h.router.Video(req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	resp, err := p.Analyze(r.Context(), &req.AnalyzeRequest)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}

// HandleGenerateVideo 处理视频生成请求
func (h *MultimodalHandler) HandleGenerateVideo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req struct {
		video.GenerateRequest
		Provider string `json:"provider,omitempty"`
	}
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, err := h.router.GenerateVideo(r.Context(), &req.GenerateRequest, req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}
// HandleSynthesize 处理文本转语音 (TTS) 请求
func (h *MultimodalHandler) HandleSynthesize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req struct {
		speech.TTSRequest
		Provider string `json:"provider,omitempty"`
	}
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, err := h.router.Synthesize(r.Context(), &req.TTSRequest, req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}

	// Buffer audio data if only stream is available
	if resp.AudioData == nil && resp.Audio != nil {
		defer resp.Audio.Close()
		data, err := io.ReadAll(resp.Audio)
		if err != nil {
			WriteError(w, types.NewInternalError("failed to read audio data"), h.logger)
			return
		}
		resp.AudioData = data
		resp.Audio = nil
	}
	WriteSuccess(w, resp)
}

// transcribeRequest is a JSON-friendly wrapper for STT requests.
// speech.STTRequest uses io.Reader which cannot be decoded from JSON.
type transcribeRequest struct {
	AudioData              string            `json:"audio_data,omitempty"`              // Base64 encoded audio
	AudioURL               string            `json:"audio_url,omitempty"`
	Model                  string            `json:"model,omitempty"`
	Language               string            `json:"language,omitempty"`
	Prompt                 string            `json:"prompt,omitempty"`
	ResponseFormat         string            `json:"response_format,omitempty"`
	Temperature            float64           `json:"temperature,omitempty"`
	TimestampGranularities []string          `json:"timestamp_granularities,omitempty"`
	Diarization            bool              `json:"diarization,omitempty"`
	Provider               string            `json:"provider,omitempty"`
	Metadata               map[string]string `json:"metadata,omitempty"`
}

// HandleTranscribe 处理语音转文本 (STT) 请求
func (h *MultimodalHandler) HandleTranscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req transcribeRequest
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	sttReq := &speech.STTRequest{
		AudioURL:               req.AudioURL,
		Model:                  req.Model,
		Language:               req.Language,
		Prompt:                 req.Prompt,
		ResponseFormat:         req.ResponseFormat,
		Temperature:            req.Temperature,
		TimestampGranularities: req.TimestampGranularities,
		Diarization:            req.Diarization,
		Metadata:               req.Metadata,
	}
	if req.AudioData != "" {
		audioBytes, err := base64.StdEncoding.DecodeString(req.AudioData)
		if err != nil {
			WriteError(w, types.NewInvalidRequestError("invalid base64 audio_data"), h.logger)
			return
		}
		sttReq.Audio = strings.NewReader(string(audioBytes))
	}

	resp, err := h.router.Transcribe(r.Context(), sttReq, req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}

// HandleGenerateMusic 处理音乐生成请求
func (h *MultimodalHandler) HandleGenerateMusic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req struct {
		music.GenerateRequest
		Provider string `json:"provider,omitempty"`
	}
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, err := h.router.GenerateMusic(r.Context(), &req.GenerateRequest, req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}

// HandleGenerate3D 处理 3D 模型生成请求
func (h *MultimodalHandler) HandleGenerate3D(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req struct {
		threed.GenerateRequest
		Provider string `json:"provider,omitempty"`
	}
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, err := h.router.Generate3D(r.Context(), &req.GenerateRequest, req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}

// HandleModerate 处理内容审核请求
func (h *MultimodalHandler) HandleModerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, types.NewInvalidRequestError("method not allowed").WithHTTPStatus(http.StatusMethodNotAllowed), h.logger)
		return
	}
	if !ValidateContentType(w, r, h.logger) {
		return
	}

	var req struct {
		moderation.ModerationRequest
		Provider string `json:"provider,omitempty"`
	}
	if err := DecodeJSONBody(w, r, &req, h.logger); err != nil {
		return
	}

	resp, err := h.router.Moderate(r.Context(), &req.ModerationRequest, req.Provider)
	if err != nil {
		h.handleMultimodalError(w, err)
		return
	}
	WriteSuccess(w, resp)
}

// handleMultimodalError converts multimodal errors to API error responses.
func (h *MultimodalHandler) handleMultimodalError(w http.ResponseWriter, err error) {
	if typedErr, ok := err.(*types.Error); ok {
		WriteError(w, typedErr, h.logger)
		return
	}
	// Provider-not-found errors from the router
	errMsg := err.Error()
	if strings.Contains(errMsg, "not found") {
		WriteError(w, types.NewNotFoundError(errMsg), h.logger)
		return
	}
	WriteError(w, types.NewInternalError(errMsg).WithRetryable(false), h.logger)
}

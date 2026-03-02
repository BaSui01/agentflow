package gateway

import (
	"strings"

	"github.com/BaSui01/agentflow/llm"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func validateRequest(req *llmcore.UnifiedRequest) *types.Error {
	if req == nil {
		return types.NewInvalidRequestError("request is required")
	}
	if strings.TrimSpace(string(req.Capability)) == "" {
		return types.NewInvalidRequestError("capability is required")
	}
	if req.Payload == nil {
		return types.NewInvalidRequestError("payload is required")
	}
	if err := validateCapabilityPayload(req); err != nil {
		return err
	}
	return nil
}

func normalizeRequest(req *llmcore.UnifiedRequest) {
	req.ProviderHint = strings.TrimSpace(req.ProviderHint)
	req.ModelHint = strings.TrimSpace(req.ModelHint)
	req.TraceID = strings.TrimSpace(req.TraceID)
	req.Hints.Normalize()
}

func validateCapabilityPayload(req *llmcore.UnifiedRequest) *types.Error {
	switch req.Capability {
	case llmcore.CapabilityChat:
		payload, ok := req.Payload.(*llm.ChatRequest)
		if !ok || payload == nil {
			return types.NewInvalidRequestError("chat payload must be *llm.ChatRequest")
		}
	case llmcore.CapabilityTools:
		payload, ok := req.Payload.(*ToolsInput)
		if !ok || payload == nil {
			return types.NewInvalidRequestError("tools payload must be *gateway.ToolsInput")
		}
	case llmcore.CapabilityImage:
		payload, ok := req.Payload.(*ImageInput)
		if !ok || payload == nil || (payload.Generate == nil && payload.Edit == nil) {
			return types.NewInvalidRequestError("image payload must include generate or edit request")
		}
	case llmcore.CapabilityVideo:
		payload, ok := req.Payload.(*VideoInput)
		if !ok || payload == nil || payload.Generate == nil {
			return types.NewInvalidRequestError("video payload must be *gateway.VideoInput with generate request")
		}
		mode := payload.Generate.Mode
		if mode == "" {
			mode = inferVideoMode(payload.Generate.Image, payload.Generate.ImageURL)
			payload.Generate.Mode = mode
		}
		if !mode.IsValid() {
			return types.NewInvalidRequestError("video mode must be text_to_video or image_to_video")
		}
		if mode == types.VideoModeImageToVideo &&
			strings.TrimSpace(payload.Generate.Image) == "" &&
			strings.TrimSpace(payload.Generate.ImageURL) == "" {
			return types.NewInvalidRequestError("image_to_video mode requires image or image_url")
		}
		if mode == types.VideoModeTextToVideo && strings.TrimSpace(payload.Generate.Prompt) == "" {
			return types.NewInvalidRequestError("text_to_video mode requires prompt")
		}
	case llmcore.CapabilityAudio:
		payload, ok := req.Payload.(*AudioInput)
		if !ok || payload == nil {
			return types.NewInvalidRequestError("audio payload must be *gateway.AudioInput")
		}
		if (payload.Synthesize == nil && payload.Transcribe == nil) ||
			(payload.Synthesize != nil && payload.Transcribe != nil) {
			return types.NewInvalidRequestError("audio payload must include exactly one of synthesize or transcribe request")
		}
		if payload.Synthesize != nil && strings.TrimSpace(payload.Synthesize.Text) == "" {
			return types.NewInvalidRequestError("tts request text is required")
		}
		if payload.Transcribe != nil && strings.TrimSpace(payload.Transcribe.AudioURL) == "" && payload.Transcribe.Audio == nil {
			return types.NewInvalidRequestError("stt request requires audio or audio_url")
		}
	case llmcore.CapabilityEmbedding:
		payload, ok := req.Payload.(*EmbeddingInput)
		if !ok || payload == nil || payload.Request == nil {
			return types.NewInvalidRequestError("embedding payload must be *gateway.EmbeddingInput with request")
		}
	case llmcore.CapabilityRerank:
		payload, ok := req.Payload.(*RerankInput)
		if !ok || payload == nil || payload.Request == nil {
			return types.NewInvalidRequestError("rerank payload must be *gateway.RerankInput with request")
		}
	case llmcore.CapabilityModeration:
		payload, ok := req.Payload.(*ModerationInput)
		if !ok || payload == nil || payload.Request == nil {
			return types.NewInvalidRequestError("moderation payload must be *gateway.ModerationInput with request")
		}
	case llmcore.CapabilityMusic:
		payload, ok := req.Payload.(*MusicInput)
		if !ok || payload == nil || payload.Generate == nil {
			return types.NewInvalidRequestError("music payload must be *gateway.MusicInput with generate request")
		}
	case llmcore.CapabilityThreeD:
		payload, ok := req.Payload.(*ThreeDInput)
		if !ok || payload == nil || payload.Generate == nil {
			return types.NewInvalidRequestError("threed payload must be *gateway.ThreeDInput with generate request")
		}
	case llmcore.CapabilityAvatar:
		payload, ok := req.Payload.(*AvatarInput)
		if !ok || payload == nil || payload.Generate == nil {
			return types.NewInvalidRequestError("avatar payload must be *gateway.AvatarInput with generate request")
		}
		mode := payload.Generate.DriveMode
		if mode == "" {
			mode = inferAvatarDriveMode(payload.Generate.AudioURL, payload.Generate.DriveVideoURL)
			payload.Generate.DriveMode = mode
		}
		if !mode.IsValid() {
			return types.NewInvalidRequestError("avatar drive_mode must be text, audio or video")
		}
		if mode == types.AvatarDriveModeText &&
			strings.TrimSpace(payload.Generate.Prompt) == "" &&
			strings.TrimSpace(payload.Generate.Script) == "" {
			return types.NewInvalidRequestError("avatar text mode requires prompt or script")
		}
		if mode == types.AvatarDriveModeAudio && strings.TrimSpace(payload.Generate.AudioURL) == "" {
			return types.NewInvalidRequestError("avatar audio mode requires audio_url")
		}
		if mode == types.AvatarDriveModeVideo && strings.TrimSpace(payload.Generate.DriveVideoURL) == "" {
			return types.NewInvalidRequestError("avatar video mode requires drive_video_url")
		}
	default:
		return nil
	}
	return nil
}

func inferVideoMode(imageB64, imageURL string) types.VideoGenerationMode {
	if strings.TrimSpace(imageB64) != "" || strings.TrimSpace(imageURL) != "" {
		return types.VideoModeImageToVideo
	}
	return types.VideoModeTextToVideo
}

func inferAvatarDriveMode(audioURL, videoURL string) types.AvatarDriveMode {
	if strings.TrimSpace(videoURL) != "" {
		return types.AvatarDriveModeVideo
	}
	if strings.TrimSpace(audioURL) != "" {
		return types.AvatarDriveModeAudio
	}
	return types.AvatarDriveModeText
}

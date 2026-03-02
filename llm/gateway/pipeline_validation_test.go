package gateway

import (
	"testing"

	speech "github.com/BaSui01/agentflow/llm/capabilities/audio"
	"github.com/BaSui01/agentflow/llm/capabilities/avatar"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func TestValidateRequest_VideoModeValidation(t *testing.T) {
	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityVideo,
		Payload: &VideoInput{
			Generate: &video.GenerateRequest{
				Mode: types.VideoModeImageToVideo,
			},
		},
	}
	if err := validateRequest(req); err == nil {
		t.Fatal("expected error for image_to_video without image input")
	}
}

func TestValidateRequest_VideoModeInference(t *testing.T) {
	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityVideo,
		Payload: &VideoInput{
			Generate: &video.GenerateRequest{
				ImageURL: "https://example.com/input.png",
			},
		},
	}
	if err := validateRequest(req); err != nil {
		t.Fatalf("expected request valid, got err=%v", err)
	}
	payload := req.Payload.(*VideoInput)
	if payload.Generate.Mode != types.VideoModeImageToVideo {
		t.Fatalf("expected inferred mode=%q, got=%q", types.VideoModeImageToVideo, payload.Generate.Mode)
	}
}

func TestValidateRequest_AvatarDriveModeValidation(t *testing.T) {
	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityAvatar,
		Payload: &AvatarInput{
			Generate: &avatar.GenerateRequest{
				DriveMode: types.AvatarDriveModeAudio,
			},
		},
	}
	if err := validateRequest(req); err == nil {
		t.Fatal("expected error for audio mode without audio_url")
	}
}

func TestValidateRequest_AvatarDriveModeInference(t *testing.T) {
	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityAvatar,
		Payload: &AvatarInput{
			Generate: &avatar.GenerateRequest{
				DriveVideoURL: "https://example.com/driver.mp4",
			},
		},
	}
	if err := validateRequest(req); err != nil {
		t.Fatalf("expected request valid, got err=%v", err)
	}
	payload := req.Payload.(*AvatarInput)
	if payload.Generate.DriveMode != types.AvatarDriveModeVideo {
		t.Fatalf("expected inferred mode=%q, got=%q", types.AvatarDriveModeVideo, payload.Generate.DriveMode)
	}
}

func TestValidateRequest_AudioRequiresSingleAction(t *testing.T) {
	req := &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityAudio,
		Payload: &AudioInput{
			Synthesize: &speech.TTSRequest{Text: "hello"},
			Transcribe: &speech.STTRequest{AudioURL: "https://example.com/audio.wav"},
		},
	}
	if err := validateRequest(req); err == nil {
		t.Fatal("expected error when synthesize/transcribe both set")
	}
}

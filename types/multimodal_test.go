package types

import "testing"

func TestVideoGenerationMode_IsValid(t *testing.T) {
	valid := []VideoGenerationMode{
		VideoModeTextToVideo,
		VideoModeImageToVideo,
	}
	for _, mode := range valid {
		if !mode.IsValid() {
			t.Fatalf("expected mode %q valid", mode)
		}
	}
	if VideoGenerationMode("invalid").IsValid() {
		t.Fatal("expected invalid mode to be rejected")
	}
}

func TestAvatarDriveMode_IsValid(t *testing.T) {
	valid := []AvatarDriveMode{
		AvatarDriveModeText,
		AvatarDriveModeAudio,
		AvatarDriveModeVideo,
	}
	for _, mode := range valid {
		if !mode.IsValid() {
			t.Fatalf("expected mode %q valid", mode)
		}
	}
	if AvatarDriveMode("invalid").IsValid() {
		t.Fatal("expected invalid mode to be rejected")
	}
}

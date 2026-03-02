package providers

import (
	"testing"
)

// expectedProviders is the canonical list of 13 chat providers in stable order.
var expectedProviders = []string{
	"OpenAI", "Claude", "Gemini", "DeepSeek", "Qwen",
	"GLM", "Grok", "Doubao", "Kimi", "Mistral",
	"Hunyuan", "MiniMax", "Llama",
}

func TestCapabilityMatrix_ContainsAll13Providers(t *testing.T) {
	if len(ChatProviderCapabilityMatrix) != len(expectedProviders) {
		t.Fatalf("expected %d providers, got %d", len(expectedProviders), len(ChatProviderCapabilityMatrix))
	}
	for i, want := range expectedProviders {
		got := ChatProviderCapabilityMatrix[i].Provider
		if got != want {
			t.Errorf("index %d: expected provider %q, got %q", i, want, got)
		}
	}
}

func TestCapabilityMatrix_NoDuplicateProviders(t *testing.T) {
	seen := make(map[string]bool)
	for _, cap := range ChatProviderCapabilityMatrix {
		if seen[cap.Provider] {
			t.Errorf("duplicate provider: %s", cap.Provider)
		}
		seen[cap.Provider] = true
	}
}

func TestCapabilityMatrix_StructFieldsNotAllFalse(t *testing.T) {
	// At least some providers must have capabilities (sanity check).
	anyTrue := false
	for _, cap := range ChatProviderCapabilityMatrix {
		if cap.Image || cap.Video || cap.AudioGenerate || cap.AudioSTT || cap.Embedding || cap.FineTuning || cap.Rerank {
			anyTrue = true
			break
		}
	}
	if !anyTrue {
		t.Error("all providers have all capabilities set to false; matrix is likely uninitialized")
	}
}

// TestCapabilityMatrix_KnownCapabilities verifies specific known-true capabilities
// that are confirmed by code inspection. If any of these fail, the matrix has drifted.
func TestCapabilityMatrix_KnownCapabilities(t *testing.T) {
	m := make(map[string]ProviderCapability)
	for _, cap := range ChatProviderCapabilityMatrix {
		m[cap.Provider] = cap
	}

	checks := []struct {
		provider string
		field    string
		want     bool
	}{
		// OpenAI: all 6 multimodal capabilities implemented
		{"OpenAI", "Image", true},
		{"OpenAI", "Video", true},
		{"OpenAI", "AudioGenerate", true},
		{"OpenAI", "AudioSTT", true},
		{"OpenAI", "Embedding", true},
		{"OpenAI", "FineTuning", true},

		// Claude: none implemented
		{"Claude", "Image", false},
		{"Claude", "Video", false},
		{"Claude", "Embedding", false},
		{"Claude", "FineTuning", false},

		// Gemini: image, video, audio, stt, embedding, fine-tuning
		{"Gemini", "Image", true},
		{"Gemini", "Video", true},
		{"Gemini", "AudioGenerate", true},
		{"Gemini", "AudioSTT", true},
		{"Gemini", "Embedding", true},
		{"Gemini", "FineTuning", true},

		// Qwen: image, video, audio, embedding, rerank
		{"Qwen", "Image", true},
		{"Qwen", "Video", true},
		{"Qwen", "AudioGenerate", true},
		{"Qwen", "Embedding", true},
		{"Qwen", "Rerank", true},

		// GLM: image, video, audio, embedding, fine-tuning, rerank
		{"GLM", "Image", true},
		{"GLM", "Video", true},
		{"GLM", "AudioGenerate", true},
		{"GLM", "Embedding", true},
		{"GLM", "FineTuning", true},
		{"GLM", "Rerank", true},

		// Grok: image, video, embedding
		{"Grok", "Image", true},
		{"Grok", "Video", true},
		{"Grok", "Embedding", true},

		// Doubao: image, video, audio, embedding
		{"Doubao", "Image", true},
		{"Doubao", "Video", true},
		{"Doubao", "AudioGenerate", true},
		{"Doubao", "Embedding", true},

		// Mistral: stt, embedding, fine-tuning
		{"Mistral", "AudioSTT", true},
		{"Mistral", "Embedding", true},
		{"Mistral", "FineTuning", true},

		// DeepSeek: none
		{"DeepSeek", "Image", false},
		{"DeepSeek", "Video", false},

		// MiniMax: audio only
		{"MiniMax", "AudioGenerate", true},
		{"MiniMax", "Image", false},
	}

	for _, c := range checks {
		cap, ok := m[c.provider]
		if !ok {
			t.Errorf("provider %q not found in matrix", c.provider)
			continue
		}
		var got bool
		switch c.field {
		case "Image":
			got = cap.Image
		case "Video":
			got = cap.Video
		case "AudioGenerate":
			got = cap.AudioGenerate
		case "AudioSTT":
			got = cap.AudioSTT
		case "Embedding":
			got = cap.Embedding
		case "FineTuning":
			got = cap.FineTuning
		case "Rerank":
			got = cap.Rerank
		default:
			t.Errorf("unknown field %q", c.field)
			continue
		}
		if got != c.want {
			t.Errorf("%s.%s: got %v, want %v", c.provider, c.field, got, c.want)
		}
	}
}

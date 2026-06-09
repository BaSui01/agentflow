package providerbase

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func TestMultimodalAdapterUnsupportedMethods(t *testing.T) {
	adapter := NewMultimodalAdapter(MultimodalAdapterConfig{ProviderName: "test-provider"})

	if _, err := adapter.GenerateImage(context.Background(), &llm.ImageGenerationRequest{}); err == nil || err.(*types.Error).Provider != "test-provider" {
		t.Fatalf("GenerateImage should return provider-specific unsupported error, got %v", err)
	}
	if _, err := adapter.GenerateVideo(context.Background(), &llm.VideoGenerationRequest{}); err == nil || err.(*types.Error).HTTPStatus != http.StatusNotImplemented {
		t.Fatalf("GenerateVideo should return 501 unsupported error, got %v", err)
	}
	if _, err := adapter.GenerateAudio(context.Background(), &llm.AudioGenerationRequest{}); err == nil {
		t.Fatalf("GenerateAudio should return unsupported error")
	}
	if _, err := adapter.TranscribeAudio(context.Background(), &llm.AudioTranscriptionRequest{}); err == nil {
		t.Fatalf("TranscribeAudio should return unsupported error")
	}
	if _, err := adapter.CreateEmbedding(context.Background(), &llm.EmbeddingRequest{}); err == nil {
		t.Fatalf("CreateEmbedding should return unsupported error")
	}
}

func TestMultimodalAdapterUnsupportedFineTuningMethods(t *testing.T) {
	adapter := NewMultimodalAdapter(MultimodalAdapterConfig{ProviderName: "test-provider"})

	if _, err := adapter.CreateFineTuningJob(context.Background(), &llm.FineTuningJobRequest{}); err == nil {
		t.Fatalf("CreateFineTuningJob should return unsupported error")
	}
	if _, err := adapter.ListFineTuningJobs(context.Background()); err == nil {
		t.Fatalf("ListFineTuningJobs should return unsupported error")
	}
	if _, err := adapter.GetFineTuningJob(context.Background(), "job-1"); err == nil {
		t.Fatalf("GetFineTuningJob should return unsupported error")
	}
	if err := adapter.CancelFineTuningJob(context.Background(), "job-1"); err == nil {
		t.Fatalf("CancelFineTuningJob should return unsupported error")
	}
}

func TestProviderMultimodalFilesDoNotReimplementUnsupportedErrors(t *testing.T) {
	providersDir := filepath.Join("..")

	var violations []string
	entries, err := os.ReadDir(providersDir)
	if err != nil {
		t.Fatalf("read providers dir: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(providersDir, entry.Name(), "multimodal.go")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "providerbase.NotSupportedError(") {
			violations = append(violations, filepath.ToSlash(path))
		}
	}
	if len(violations) > 0 {
		t.Fatalf("provider multimodal files must delegate unsupported capabilities to MultimodalAdapter, found direct NotSupportedError calls in: %s", strings.Join(violations, ", "))
	}
}

package agentflow_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGoogleGeminiEndpointPathsStayOutOfHTTPEntryLayers(t *testing.T) {
	targetDirs := []string{
		filepath.FromSlash("api/routes"),
		filepath.FromSlash("api/handlers"),
		filepath.FromSlash("cmd/agentflow"),
		filepath.FromSlash("internal/app/bootstrap"),
	}
	forbiddenSnippets := []string{
		":generateContent",
		":streamGenerateContent",
		"/v1beta/models/",
		"/publishers/google/models/",
		"google.golang.org/genai",
	}

	for _, dir := range targetDirs {
		walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(data)
			for _, snippet := range forbiddenSnippets {
				if strings.Contains(text, snippet) {
					t.Fatalf("%s must not hardcode Google/Gemini endpoint or SDK detail %q; keep it in llm/providers or llm/internal/googlegenai", path, snippet)
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("scan %s: %v", dir, walkErr)
		}
	}
}

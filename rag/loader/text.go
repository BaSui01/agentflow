package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BaSui01/agentflow/rag"
)

// TextLoader loads plain text files as a single Document.
type TextLoader struct{}

// NewTextLoader creates a TextLoader.
func NewTextLoader() *TextLoader {
	return &TextLoader{}
}

// Load reads a text file and returns it as a single Document.
func (l *TextLoader) Load(ctx context.Context, source string) ([]rag.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("text loader: %w", err)
	}

	doc := rag.Document{
		ID:      source,
		Content: string(data),
		Metadata: map[string]any{
			"source_file":  filepath.Base(source),
			"source_path":  source,
			"content_type": "text/plain",
			"loader":       "text",
		},
	}

	return []rag.Document{doc}, nil
}

// SupportedTypes returns the extensions handled by TextLoader.
func (l *TextLoader) SupportedTypes() []string {
	return []string{".txt"}
}

package loader

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	rag "github.com/BaSui01/agentflow/rag/runtime"
)

type PDFLoader struct{}

func NewPDFLoader() *PDFLoader {
	return &PDFLoader{}
}

func (l *PDFLoader) Load(ctx context.Context, source string) ([]rag.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// X-011: path normalization and traversal check
	clean := filepath.Clean(source)
	if strings.Contains(clean, "..") {
		return nil, fmt.Errorf("pdf loader: source path must not contain ..")
	}

	data, err := os.ReadFile(clean)
	if err != nil {
		return nil, fmt.Errorf("pdf loader: %w", err)
	}

	text, err := l.extractText(clean, data)
	if err != nil {
		return nil, err
	}

	doc := rag.Document{
		ID:      clean,
		Content: strings.TrimSpace(text),
		Metadata: map[string]any{
			"source_file":  filepath.Base(clean),
			"source_path":  clean,
			"content_type": "application/pdf",
			"loader":       "pdf",
		},
	}
	return []rag.Document{doc}, nil
}

func (l *PDFLoader) extractText(source string, data []byte) (string, error) {
	if _, err := exec.LookPath("pdftotext"); err == nil {
		cmd := exec.CommandContext(context.Background(), "pdftotext", "-layout", source, "-")
		cmd.Stdin = nil
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}
	return l.fallbackExtract(data), nil
}

func (l *PDFLoader) fallbackExtract(data []byte) string {
	var sb strings.Builder
	inText := false
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b >= 32 && b < 127 {
			sb.WriteByte(b)
			inText = true
		} else if b == '\n' || b == '\r' || b == '\t' {
			if inText {
				sb.WriteByte(' ')
				inText = false
			}
		}
	}
	return sb.String()
}

func (l *PDFLoader) SupportedTypes() []string {
	return []string{".pdf"}
}

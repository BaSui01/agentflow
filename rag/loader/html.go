package loader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BaSui01/agentflow/rag"
	"golang.org/x/net/html"
)

type HTMLLoader struct{}

func NewHTMLLoader() *HTMLLoader {
	return &HTMLLoader{}
}

func (l *HTMLLoader) Load(ctx context.Context, source string) ([]rag.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("html loader: %w", err)
	}

	text := l.extractText(data)
	doc := rag.Document{
		ID:      source,
		Content: strings.TrimSpace(text),
		Metadata: map[string]any{
			"source_file":  filepath.Base(source),
			"source_path":  source,
			"content_type": "text/html",
			"loader":       "html",
		},
	}
	return []rag.Document{doc}, nil
}

var textTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"li": true, "td": true, "th": true, "blockquote": true, "div": true, "span": true,
}

var skipTags = map[string]bool{
	"script": true, "style": true, "noscript": true,
}

func (l *HTMLLoader) extractText(data []byte) string {
	doc, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		return ""
	}
	var sb strings.Builder
	l.walk(doc, &sb)
	return sb.String()
}

func (l *HTMLLoader) walk(n *html.Node, sb *strings.Builder) {
	if n == nil {
		return
	}
	if n.Type == html.TextNode {
		t := strings.TrimSpace(n.Data)
		if t != "" {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(t)
		}
		return
	}
	if n.Type == html.ElementNode && skipTags[strings.ToLower(n.Data)] {
		return
	}
	if n.Type == html.ElementNode && textTags[strings.ToLower(n.Data)] && sb.Len() > 0 {
		sb.WriteByte('\n')
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		l.walk(c, sb)
	}
}

func (l *HTMLLoader) SupportedTypes() []string {
	return []string{".html", ".htm"}
}

package loader

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BaSui01/agentflow/rag"
)

// MarkdownLoader loads Markdown files, splitting by top-level headings.
// Each heading section becomes a separate Document with the heading preserved in metadata.
// If the file has no headings, the entire content is returned as a single Document.
type MarkdownLoader struct{}

// NewMarkdownLoader creates a MarkdownLoader.
func NewMarkdownLoader() *MarkdownLoader {
	return &MarkdownLoader{}
}

// Load reads a Markdown file and splits it into Documents by heading.
func (l *MarkdownLoader) Load(ctx context.Context, source string) ([]rag.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("markdown loader: %w", err)
	}
	defer f.Close()

	baseName := filepath.Base(source)

	type section struct {
		heading string
		level   int
		lines   []string
	}

	var sections []section
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Text()
		if heading, level := parseHeading(line); heading != "" {
			sections = append(sections, section{heading: heading, level: level})
		} else {
			if len(sections) == 0 {
				// Content before any heading goes into a preamble section.
				sections = append(sections, section{heading: "", level: 0})
			}
			sections[len(sections)-1].lines = append(sections[len(sections)-1].lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("markdown loader: reading %s: %w", source, err)
	}

	// If no sections were found (empty file), return empty slice.
	if len(sections) == 0 {
		return []rag.Document{}, nil
	}

	// If there is only one section (no headings or single heading), return as one doc.
	docs := make([]rag.Document, 0, len(sections))
	for i, sec := range sections {
		content := strings.TrimSpace(strings.Join(sec.lines, "\n"))
		if content == "" && sec.heading == "" {
			continue
		}

		meta := map[string]any{
			"source_file":  baseName,
			"source_path":  source,
			"content_type": "text/markdown",
			"loader":       "markdown",
			"section":      i,
		}
		if sec.heading != "" {
			meta["heading"] = sec.heading
			meta["heading_level"] = sec.level
		}

		doc := rag.Document{
			ID:       fmt.Sprintf("%s#%d", source, i),
			Content:  content,
			Metadata: meta,
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// parseHeading detects ATX-style headings (# Heading).
// Returns the heading text and level (1-6), or ("", 0) if not a heading.
func parseHeading(line string) (heading string, level int) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "#") {
		return "", 0
	}
	level = 0
	for _, ch := range trimmed {
		if ch == '#' {
			level++
		} else {
			break
		}
	}
	if level < 1 || level > 6 {
		return "", 0
	}
	heading = strings.TrimSpace(trimmed[level:])
	if heading == "" {
		return "", 0
	}
	return heading, level
}

// SupportedTypes returns the extensions handled by MarkdownLoader.
func (l *MarkdownLoader) SupportedTypes() []string {
	return []string{".md"}
}

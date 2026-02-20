package loader

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BaSui01/agentflow/rag"
)

// JSONLoaderConfig configures the JSON/JSONL loader.
type JSONLoaderConfig struct {
	// ContentField is the JSON field name to use as Document.Content.
	// If empty, the entire JSON object is serialized as content.
	ContentField string
	// IDField is the JSON field name to use as Document.ID.
	// If empty, a path-based ID is generated.
	IDField string
}

// JSONLoader loads JSON (single object or array) and JSONL files.
type JSONLoader struct {
	config JSONLoaderConfig
}

// NewJSONLoader creates a JSONLoader.
func NewJSONLoader(config JSONLoaderConfig) *JSONLoader {
	return &JSONLoader{config: config}
}

// Load reads a JSON or JSONL file and returns Documents.
func (l *JSONLoader) Load(ctx context.Context, source string) ([]rag.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(source))
	if ext == ".jsonl" {
		return l.loadJSONL(source)
	}
	return l.loadJSON(source)
}

// loadJSON handles .json files (single object or array).
func (l *JSONLoader) loadJSON(source string) ([]rag.Document, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("json loader: %w", err)
	}

	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return []rag.Document{}, nil
	}

	// Try array first, then single object.
	if data[0] == '[' {
		var items []map[string]any
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("json loader: parsing array in %s: %w", source, err)
		}
		return l.objectsToDocs(source, items), nil
	}

	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("json loader: parsing object in %s: %w", source, err)
	}
	return l.objectsToDocs(source, []map[string]any{obj}), nil
}

// loadJSONL handles .jsonl files (one JSON object per line).
func (l *JSONLoader) loadJSONL(source string) ([]rag.Document, error) {
	f, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("jsonl loader: %w", err)
	}
	defer f.Close()

	var items []map[string]any
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return nil, fmt.Errorf("jsonl loader: line %d in %s: %w", lineNum, source, err)
		}
		items = append(items, obj)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("jsonl loader: reading %s: %w", source, err)
	}

	return l.objectsToDocs(source, items), nil
}

// objectsToDocs converts parsed JSON objects into Documents.
func (l *JSONLoader) objectsToDocs(source string, items []map[string]any) []rag.Document {
	baseName := filepath.Base(source)
	docs := make([]rag.Document, 0, len(items))

	for i, obj := range items {
		content := l.extractContent(obj)
		id := l.extractID(obj, source, i)

		doc := rag.Document{
			ID:      id,
			Content: content,
			Metadata: map[string]any{
				"source_file":  baseName,
				"source_path":  source,
				"content_type": "application/json",
				"loader":       "json",
				"index":        i,
			},
		}
		docs = append(docs, doc)
	}
	return docs
}

// extractContent gets the content string from a JSON object.
func (l *JSONLoader) extractContent(obj map[string]any) string {
	if l.config.ContentField != "" {
		if val, ok := obj[l.config.ContentField]; ok {
			return fmt.Sprintf("%v", val)
		}
	}
	// Fallback: serialize the whole object.
	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("%v", obj)
	}
	return string(data)
}

// extractID gets the ID from a JSON object or generates one.
func (l *JSONLoader) extractID(obj map[string]any, source string, index int) string {
	if l.config.IDField != "" {
		if val, ok := obj[l.config.IDField]; ok {
			return fmt.Sprintf("%v", val)
		}
	}
	return fmt.Sprintf("%s#%d", source, index)
}

// SupportedTypes returns the extensions handled by JSONLoader.
func (l *JSONLoader) SupportedTypes() []string {
	return []string{".json", ".jsonl"}
}

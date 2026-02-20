package loader

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BaSui01/agentflow/rag"
)

// CSVLoaderConfig configures the CSV loader.
type CSVLoaderConfig struct {
	// Delimiter is the field separator. Defaults to ','.
	Delimiter rune
	// RowsPerDocument controls how many rows are grouped into a single Document.
	// 0 or 1 means each row becomes its own Document.
	RowsPerDocument int
	// ContentColumns lists column names (from the header) to include in Document.Content.
	// If empty, all columns are concatenated.
	ContentColumns []string
}

// CSVLoader loads CSV files. Each row (or group of rows) becomes a Document.
// The first row is treated as a header.
type CSVLoader struct {
	config CSVLoaderConfig
}

// NewCSVLoader creates a CSVLoader with the given config.
func NewCSVLoader(config CSVLoaderConfig) *CSVLoader {
	if config.Delimiter == 0 {
		config.Delimiter = ','
	}
	if config.RowsPerDocument <= 0 {
		config.RowsPerDocument = 1
	}
	return &CSVLoader{config: config}
}

// Load reads a CSV file and returns Documents.
func (l *CSVLoader) Load(ctx context.Context, source string) ([]rag.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("csv loader: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.Comma = l.config.Delimiter
	reader.LazyQuotes = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv loader: parsing %s: %w", source, err)
	}

	if len(records) < 2 {
		// Only header or empty file.
		return []rag.Document{}, nil
	}

	header := records[0]
	dataRows := records[1:]
	baseName := filepath.Base(source)

	// Determine which column indices to use for content.
	contentIndices := l.resolveContentColumns(header)

	var docs []rag.Document
	for i := 0; i < len(dataRows); i += l.config.RowsPerDocument {
		end := i + l.config.RowsPerDocument
		if end > len(dataRows) {
			end = len(dataRows)
		}
		chunk := dataRows[i:end]

		var contentParts []string
		for _, row := range chunk {
			var parts []string
			for _, idx := range contentIndices {
				if idx < len(row) {
					parts = append(parts, row[idx])
				}
			}
			contentParts = append(contentParts, strings.Join(parts, " "))
		}

		doc := rag.Document{
			ID:      fmt.Sprintf("%s#row%d", source, i),
			Content: strings.Join(contentParts, "\n"),
			Metadata: map[string]any{
				"source_file":  baseName,
				"source_path":  source,
				"content_type": "text/csv",
				"loader":       "csv",
				"row_start":    i,
				"row_end":      end - 1,
				"columns":      header,
			},
		}
		docs = append(docs, doc)
	}

	return docs, nil
}

// resolveContentColumns returns column indices to include in content.
func (l *CSVLoader) resolveContentColumns(header []string) []int {
	if len(l.config.ContentColumns) == 0 {
		indices := make([]int, len(header))
		for i := range header {
			indices[i] = i
		}
		return indices
	}

	wanted := make(map[string]bool, len(l.config.ContentColumns))
	for _, col := range l.config.ContentColumns {
		wanted[strings.ToLower(col)] = true
	}

	var indices []int
	for i, h := range header {
		if wanted[strings.ToLower(h)] {
			indices = append(indices, i)
		}
	}
	// Fallback: if no columns matched, use all.
	if len(indices) == 0 {
		indices = make([]int, len(header))
		for i := range header {
			indices[i] = i
		}
	}
	return indices
}

// SupportedTypes returns the extensions handled by CSVLoader.
func (l *CSVLoader) SupportedTypes() []string {
	return []string{".csv"}
}

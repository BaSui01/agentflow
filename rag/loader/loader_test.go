package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/BaSui01/agentflow/rag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// LoaderRegistry Tests
// ============================================================

func TestNewLoaderRegistry_HasBuiltinLoaders(t *testing.T) {
	t.Parallel()

	r := NewLoaderRegistry()
	types := r.SupportedTypes()

	assert.Contains(t, types, ".txt")
	assert.Contains(t, types, ".md")
	assert.Contains(t, types, ".csv")
	assert.Contains(t, types, ".json")
	assert.Contains(t, types, ".jsonl")
}

func TestLoaderRegistry_Register_CustomLoader(t *testing.T) {
	t.Parallel()

	r := NewLoaderRegistry()
	r.Register(".xml", NewTextLoader()) // reuse text loader for test

	assert.Contains(t, r.SupportedTypes(), ".xml")
}

func TestLoaderRegistry_Load_NoExtension(t *testing.T) {
	t.Parallel()

	r := NewLoaderRegistry()
	_, err := r.Load(context.Background(), "noextension")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no extension")
}

func TestLoaderRegistry_Load_UnknownExtension(t *testing.T) {
	t.Parallel()

	r := NewLoaderRegistry()
	_, err := r.Load(context.Background(), "file.xyz")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no loader registered")
}

func TestLoaderRegistry_Load_CaseInsensitive(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.TXT")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0o644))

	r := NewLoaderRegistry()
	docs, err := r.Load(context.Background(), path)

	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "hello", docs[0].Content)
}

// ============================================================
// TextLoader Tests
// ============================================================

func TestTextLoader_Load(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.txt")
	content := "Hello, world!\nSecond line."
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	loader := NewTextLoader()
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, content, docs[0].Content)
	assert.Equal(t, path, docs[0].ID)
	assert.Equal(t, "text/plain", docs[0].Metadata["content_type"])
	assert.Equal(t, "sample.txt", docs[0].Metadata["source_file"])
	assert.Equal(t, "text", docs[0].Metadata["loader"])
}

func TestTextLoader_Load_FileNotFound(t *testing.T) {
	t.Parallel()

	loader := NewTextLoader()
	_, err := loader.Load(context.Background(), "/nonexistent/file.txt")

	assert.Error(t, err)
}

func TestTextLoader_Load_CancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	loader := NewTextLoader()
	_, err := loader.Load(ctx, "any.txt")

	assert.ErrorIs(t, err, context.Canceled)
}

func TestTextLoader_SupportedTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{".txt"}, NewTextLoader().SupportedTypes())
}

// ============================================================
// MarkdownLoader Tests
// ============================================================

func TestMarkdownLoader_Load_WithHeadings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	md := "# Introduction\nSome intro text.\n\n## Details\nDetail content here."
	require.NoError(t, os.WriteFile(path, []byte(md), 0o644))

	loader := NewMarkdownLoader()
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 2)

	assert.Equal(t, "Introduction", docs[0].Metadata["heading"])
	assert.Equal(t, 1, docs[0].Metadata["heading_level"])
	assert.Contains(t, docs[0].Content, "Some intro text.")

	assert.Equal(t, "Details", docs[1].Metadata["heading"])
	assert.Equal(t, 2, docs[1].Metadata["heading_level"])
	assert.Contains(t, docs[1].Content, "Detail content here.")
}

func TestMarkdownLoader_Load_NoHeadings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "plain.md")
	require.NoError(t, os.WriteFile(path, []byte("Just plain text.\nNo headings."), 0o644))

	loader := NewMarkdownLoader()
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Contains(t, docs[0].Content, "Just plain text.")
	assert.Equal(t, "text/markdown", docs[0].Metadata["content_type"])
}

func TestMarkdownLoader_Load_EmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	loader := NewMarkdownLoader()
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestMarkdownLoader_SupportedTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{".md"}, NewMarkdownLoader().SupportedTypes())
}

func TestParseHeading(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		line          string
		expectHeading string
		expectLevel   int
	}{
		{"h1", "# Title", "Title", 1},
		{"h2", "## Section", "Section", 2},
		{"h3", "### Sub", "Sub", 3},
		{"not heading", "regular text", "", 0},
		{"hash only", "#", "", 0},
		{"too many hashes", "####### Seven", "", 0},
		{"with spaces", "  ## Indented  ", "Indented", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			heading, level := parseHeading(tt.line)
			assert.Equal(t, tt.expectHeading, heading)
			assert.Equal(t, tt.expectLevel, level)
		})
	}
}

// ============================================================
// CSVLoader Tests
// ============================================================

func TestCSVLoader_Load_Basic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csvContent := "name,age,city\nAlice,30,NYC\nBob,25,LA"
	require.NoError(t, os.WriteFile(path, []byte(csvContent), 0o644))

	loader := NewCSVLoader(CSVLoaderConfig{})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Contains(t, docs[0].Content, "Alice")
	assert.Contains(t, docs[0].Content, "30")
	assert.Contains(t, docs[0].Content, "NYC")
	assert.Equal(t, "text/csv", docs[0].Metadata["content_type"])
}

func TestCSVLoader_Load_ContentColumns(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csvContent := "id,text,label\n1,hello world,positive\n2,goodbye,negative"
	require.NoError(t, os.WriteFile(path, []byte(csvContent), 0o644))

	loader := NewCSVLoader(CSVLoaderConfig{ContentColumns: []string{"text"}})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Equal(t, "hello world", docs[0].Content)
	assert.Equal(t, "goodbye", docs[1].Content)
}

func TestCSVLoader_Load_RowsPerDocument(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csvContent := "col\nA\nB\nC\nD\nE"
	require.NoError(t, os.WriteFile(path, []byte(csvContent), 0o644))

	loader := NewCSVLoader(CSVLoaderConfig{RowsPerDocument: 2})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	assert.Len(t, docs, 3) // [A,B], [C,D], [E]
}

func TestCSVLoader_Load_HeaderOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.csv")
	require.NoError(t, os.WriteFile(path, []byte("col1,col2\n"), 0o644))

	loader := NewCSVLoader(CSVLoaderConfig{})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestCSVLoader_Load_CustomDelimiter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	csvContent := "name\tage\nAlice\t30"
	require.NoError(t, os.WriteFile(path, []byte(csvContent), 0o644))

	loader := NewCSVLoader(CSVLoaderConfig{Delimiter: '\t'})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Contains(t, docs[0].Content, "Alice")
}

func TestCSVLoader_SupportedTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{".csv"}, NewCSVLoader(CSVLoaderConfig{}).SupportedTypes())
}

// ============================================================
// JSONLoader Tests
// ============================================================

func TestJSONLoader_Load_Array(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	jsonContent := `[{"text":"hello","id":"1"},{"text":"world","id":"2"}]`
	require.NoError(t, os.WriteFile(path, []byte(jsonContent), 0o644))

	loader := NewJSONLoader(JSONLoaderConfig{ContentField: "text", IDField: "id"})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Equal(t, "hello", docs[0].Content)
	assert.Equal(t, "1", docs[0].ID)
	assert.Equal(t, "world", docs[1].Content)
	assert.Equal(t, "2", docs[1].ID)
}

func TestJSONLoader_Load_SingleObject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "single.json")
	jsonContent := `{"content":"single doc","key":"val"}`
	require.NoError(t, os.WriteFile(path, []byte(jsonContent), 0o644))

	loader := NewJSONLoader(JSONLoaderConfig{ContentField: "content"})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "single doc", docs[0].Content)
}

func TestJSONLoader_Load_NoContentField_SerializesAll(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	jsonContent := `{"a":1,"b":"two"}`
	require.NoError(t, os.WriteFile(path, []byte(jsonContent), 0o644))

	loader := NewJSONLoader(JSONLoaderConfig{})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 1)
	// Content should be the serialized JSON.
	assert.Contains(t, docs[0].Content, `"a"`)
	assert.Contains(t, docs[0].Content, `"b"`)
}

func TestJSONLoader_Load_JSONL(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.jsonl")
	jsonlContent := "{\"text\":\"line1\"}\n{\"text\":\"line2\"}\n\n{\"text\":\"line3\"}"
	require.NoError(t, os.WriteFile(path, []byte(jsonlContent), 0o644))

	loader := NewJSONLoader(JSONLoaderConfig{ContentField: "text"})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	require.Len(t, docs, 3)
	assert.Equal(t, "line1", docs[0].Content)
	assert.Equal(t, "line2", docs[1].Content)
	assert.Equal(t, "line3", docs[2].Content)
}

func TestJSONLoader_Load_EmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	require.NoError(t, os.WriteFile(path, []byte(""), 0o644))

	loader := NewJSONLoader(JSONLoaderConfig{})
	docs, err := loader.Load(context.Background(), path)

	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestJSONLoader_Load_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	require.NoError(t, os.WriteFile(path, []byte("{invalid"), 0o644))

	loader := NewJSONLoader(JSONLoaderConfig{})
	_, err := loader.Load(context.Background(), path)

	assert.Error(t, err)
}

func TestJSONLoader_SupportedTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{".json", ".jsonl"}, NewJSONLoader(JSONLoaderConfig{}).SupportedTypes())
}

// ============================================================
// Adapter Tests (compile-time interface compliance)
// ============================================================

func TestGitHubSourceAdapter_ImplementsDocumentLoader(t *testing.T) {
	var _ DocumentLoader = (*GitHubSourceAdapter)(nil)
}

func TestArxivSourceAdapter_ImplementsDocumentLoader(t *testing.T) {
	var _ DocumentLoader = (*ArxivSourceAdapter)(nil)
}

// ============================================================
// Integration: Registry routes to correct loader
// ============================================================

func TestLoaderRegistry_Integration_LoadsCorrectFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	registry := NewLoaderRegistry()

	tests := []struct {
		name    string
		file    string
		content string
		check   func(t *testing.T, docs []rag.Document)
	}{
		{
			name:    "txt file",
			file:    "test.txt",
			content: "plain text",
			check: func(t *testing.T, docs []rag.Document) {
				t.Helper()
				require.Len(t, docs, 1)
				assert.Equal(t, "plain text", docs[0].Content)
				assert.Equal(t, "text", docs[0].Metadata["loader"])
			},
		},
		{
			name:    "md file",
			file:    "test.md",
			content: "# Title\nBody text",
			check: func(t *testing.T, docs []rag.Document) {
				t.Helper()
				require.Len(t, docs, 1)
				assert.Equal(t, "markdown", docs[0].Metadata["loader"])
			},
		},
		{
			name:    "csv file",
			file:    "test.csv",
			content: "col\nval",
			check: func(t *testing.T, docs []rag.Document) {
				t.Helper()
				require.Len(t, docs, 1)
				assert.Equal(t, "csv", docs[0].Metadata["loader"])
			},
		},
		{
			name:    "json file",
			file:    "test.json",
			content: `{"key":"value"}`,
			check: func(t *testing.T, docs []rag.Document) {
				t.Helper()
				require.Len(t, docs, 1)
				assert.Equal(t, "json", docs[0].Metadata["loader"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.file)
			require.NoError(t, os.WriteFile(path, []byte(tt.content), 0o644))

			docs, err := registry.Load(context.Background(), path)
			require.NoError(t, err)
			tt.check(t, docs)
		})
	}
}

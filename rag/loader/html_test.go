package loader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTMLLoader_SupportedTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{".html", ".htm"}, NewHTMLLoader().SupportedTypes())
}

func TestHTMLLoader_Load_FileNotFound(t *testing.T) {
	t.Parallel()
	loader := NewHTMLLoader()
	_, err := loader.Load(context.Background(), "/nonexistent/file.html")
	assert.Error(t, err)
}

func TestHTMLLoader_Load_ExtractsText(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	html := `<!DOCTYPE html><html><head><title>Test</title></head><body>
<p>First paragraph.</p>
<h1>Heading</h1>
<p>Second paragraph.</p>
</body></html>`
	require.NoError(t, os.WriteFile(path, []byte(html), 0o644))

	loader := NewHTMLLoader()
	docs, err := loader.Load(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, path, docs[0].ID)
	assert.Equal(t, "text/html", docs[0].Metadata["content_type"])
	assert.Equal(t, "html", docs[0].Metadata["loader"])
	assert.Contains(t, docs[0].Content, "First paragraph")
	assert.Contains(t, docs[0].Content, "Heading")
	assert.Contains(t, docs[0].Content, "Second paragraph")
}

func TestHTMLLoader_Load_SkipsScriptAndStyle(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	html := `<html><body><p>Visible</p><script>alert("x")</script><style>.x{}</style><p>Also visible</p></body></html>`
	require.NoError(t, os.WriteFile(path, []byte(html), 0o644))

	loader := NewHTMLLoader()
	docs, err := loader.Load(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Contains(t, docs[0].Content, "Visible")
	assert.Contains(t, docs[0].Content, "Also visible")
	assert.NotContains(t, docs[0].Content, "alert")
	assert.NotContains(t, docs[0].Content, ".x{}")
}

func TestHTMLLoader_Load_ListAndTable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "page.html")
	html := `<html><body><ul><li>Item A</li><li>Item B</li></ul><table><tr><td>Cell 1</td><td>Cell 2</td></tr></table></body></html>`
	require.NoError(t, os.WriteFile(path, []byte(html), 0o644))

	loader := NewHTMLLoader()
	docs, err := loader.Load(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Contains(t, docs[0].Content, "Item A")
	assert.Contains(t, docs[0].Content, "Item B")
	assert.Contains(t, docs[0].Content, "Cell 1")
	assert.Contains(t, docs[0].Content, "Cell 2")
}

func TestHTMLLoader_Load_HTMExtension(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "page.htm")
	require.NoError(t, os.WriteFile(path, []byte("<html><body><p>Content</p></body></html>"), 0o644))

	loader := NewHTMLLoader()
	docs, err := loader.Load(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.True(t, strings.Contains(docs[0].Content, "Content"))
}

func TestHTMLLoader_Load_CancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.html")
	require.NoError(t, os.WriteFile(path, []byte("<html></html>"), 0o644))

	loader := NewHTMLLoader()
	_, err := loader.Load(ctx, path)
	assert.ErrorIs(t, err, context.Canceled)
}

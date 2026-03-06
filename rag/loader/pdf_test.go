package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPDFLoader_SupportedTypes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{".pdf"}, NewPDFLoader().SupportedTypes())
}

func TestPDFLoader_Load_FileNotFound(t *testing.T) {
	t.Parallel()
	loader := NewPDFLoader()
	_, err := loader.Load(context.Background(), "/nonexistent/file.pdf")
	assert.Error(t, err)
}

func TestPDFLoader_Load_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.pdf")
	require.NoError(t, os.WriteFile(path, []byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"), 0o644))

	loader := NewPDFLoader()
	docs, err := loader.Load(context.Background(), path)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, path, docs[0].ID)
	assert.Equal(t, "application/pdf", docs[0].Metadata["content_type"])
	assert.Equal(t, "pdf", docs[0].Metadata["loader"])
}

func TestPDFLoader_Load_CancelledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))

	loader := NewPDFLoader()
	_, err := loader.Load(ctx, path)
	assert.ErrorIs(t, err, context.Canceled)
}

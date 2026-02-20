package loader

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/BaSui01/agentflow/rag"
)

// DocumentLoader is the unified interface for loading documents from any source.
type DocumentLoader interface {
	// Load reads the source and returns documents.
	// source is typically a file path, but loaders may interpret it as a URL or query.
	Load(ctx context.Context, source string) ([]rag.Document, error)

	// SupportedTypes returns the file extensions this loader handles (e.g. ".txt", ".md").
	SupportedTypes() []string
}

// LoaderRegistry routes Load calls to the appropriate DocumentLoader based on file extension.
type LoaderRegistry struct {
	mu      sync.RWMutex
	loaders map[string]DocumentLoader // extension (lowercase, with dot) -> loader
}

// NewLoaderRegistry creates a registry pre-populated with the built-in loaders.
func NewLoaderRegistry() *LoaderRegistry {
	r := &LoaderRegistry{
		loaders: make(map[string]DocumentLoader),
	}

	// Register built-in loaders.
	builtins := []DocumentLoader{
		NewTextLoader(),
		NewMarkdownLoader(),
		NewCSVLoader(CSVLoaderConfig{}),
		NewJSONLoader(JSONLoaderConfig{}),
	}
	for _, l := range builtins {
		for _, ext := range l.SupportedTypes() {
			r.loaders[strings.ToLower(ext)] = l
		}
	}

	return r
}

// Register adds or replaces a loader for the given file extension.
// ext should include the leading dot (e.g. ".pdf").
func (r *LoaderRegistry) Register(ext string, loader DocumentLoader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loaders[strings.ToLower(ext)] = loader
}

// Load determines the loader from the source's file extension and delegates to it.
func (r *LoaderRegistry) Load(ctx context.Context, source string) ([]rag.Document, error) {
	ext := strings.ToLower(filepath.Ext(source))
	if ext == "" {
		return nil, fmt.Errorf("loader: cannot determine file type for %q (no extension)", source)
	}

	r.mu.RLock()
	l, ok := r.loaders[ext]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("loader: no loader registered for extension %q", ext)
	}

	return l.Load(ctx, source)
}

// SupportedTypes returns all registered extensions, sorted.
func (r *LoaderRegistry) SupportedTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	exts := make([]string, 0, len(r.loaders))
	for ext := range r.loaders {
		exts = append(exts, ext)
	}
	sort.Strings(exts)
	return exts
}

package multimodal

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/llm/capabilities/audio"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock providers for Router tests ---

type mockEmbeddingProvider struct{ name string }

func (m *mockEmbeddingProvider) Embed(_ context.Context, _ *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	return &embedding.EmbeddingResponse{Provider: m.name}, nil
}
func (m *mockEmbeddingProvider) EmbedQuery(_ context.Context, _ string) ([]float64, error) {
	return []float64{0.1}, nil
}
func (m *mockEmbeddingProvider) EmbedDocuments(_ context.Context, _ []string) ([][]float64, error) {
	return nil, nil
}
func (m *mockEmbeddingProvider) Name() string       { return m.name }
func (m *mockEmbeddingProvider) Dimensions() int    { return 1536 }
func (m *mockEmbeddingProvider) MaxBatchSize() int  { return 100 }

type mockRerankProvider struct{ name string }

func (m *mockRerankProvider) Rerank(_ context.Context, _ *rerank.RerankRequest) (*rerank.RerankResponse, error) {
	return &rerank.RerankResponse{Provider: m.name}, nil
}
func (m *mockRerankProvider) RerankSimple(_ context.Context, _ string, _ []string, _ int) ([]rerank.RerankResult, error) {
	return nil, nil
}
func (m *mockRerankProvider) Name() string      { return m.name }
func (m *mockRerankProvider) MaxDocuments() int { return 100 }

type mockTTSProvider struct{ name string }

func (m *mockTTSProvider) Synthesize(_ context.Context, _ *speech.TTSRequest) (*speech.TTSResponse, error) {
	return &speech.TTSResponse{Provider: m.name}, nil
}
func (m *mockTTSProvider) SynthesizeToFile(_ context.Context, _ *speech.TTSRequest, _ string) error {
	return nil
}
func (m *mockTTSProvider) ListVoices(_ context.Context) ([]speech.Voice, error) { return nil, nil }
func (m *mockTTSProvider) Name() string                                         { return m.name }

type mockSTTProvider struct{ name string }

func (m *mockSTTProvider) Transcribe(_ context.Context, _ *speech.STTRequest) (*speech.STTResponse, error) {
	return &speech.STTResponse{Provider: m.name}, nil
}
func (m *mockSTTProvider) TranscribeFile(_ context.Context, _ string, _ *speech.STTRequest) (*speech.STTResponse, error) {
	return &speech.STTResponse{Provider: m.name}, nil
}
func (m *mockSTTProvider) Name() string                { return m.name }
func (m *mockSTTProvider) SupportedFormats() []string  { return []string{"mp3"} }

type mockImageProvider struct{ name string }

func (m *mockImageProvider) Generate(_ context.Context, _ *image.GenerateRequest) (*image.GenerateResponse, error) {
	return &image.GenerateResponse{Provider: m.name}, nil
}
func (m *mockImageProvider) Edit(_ context.Context, _ *image.EditRequest) (*image.GenerateResponse, error) {
	return nil, nil
}
func (m *mockImageProvider) CreateVariation(_ context.Context, _ *image.VariationRequest) (*image.GenerateResponse, error) {
	return nil, nil
}
func (m *mockImageProvider) Name() string              { return m.name }
func (m *mockImageProvider) SupportedSizes() []string  { return []string{"1024x1024"} }

type mockVideoProvider struct{ name string }

func (m *mockVideoProvider) Analyze(_ context.Context, _ *video.AnalyzeRequest) (*video.AnalyzeResponse, error) {
	return nil, nil
}
func (m *mockVideoProvider) Generate(_ context.Context, _ *video.GenerateRequest) (*video.GenerateResponse, error) {
	return &video.GenerateResponse{Provider: m.name}, nil
}
func (m *mockVideoProvider) Name() string                       { return m.name }
func (m *mockVideoProvider) SupportedFormats() []video.VideoFormat { return []video.VideoFormat{"mp4"} }
func (m *mockVideoProvider) SupportsGeneration() bool           { return true }

// --- Router tests ---

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	require.NotNil(t, r)
}

func TestRouter_RegisterAndGetEmbedding(t *testing.T) {
	r := NewRouter()
	p := &mockEmbeddingProvider{name: "test-embed"}
	r.RegisterEmbedding("test-embed", p, true)

	got, err := r.Embedding("")
	require.NoError(t, err)
	assert.Equal(t, "test-embed", got.Name())

	got, err = r.Embedding("test-embed")
	require.NoError(t, err)
	assert.Equal(t, "test-embed", got.Name())

	_, err = r.Embedding("nonexistent")
	assert.Error(t, err)
}

func TestRouter_RegisterAndGetRerank(t *testing.T) {
	r := NewRouter()
	p := &mockRerankProvider{name: "test-rerank"}
	r.RegisterRerank("test-rerank", p, true)

	got, err := r.Rerank("")
	require.NoError(t, err)
	assert.Equal(t, "test-rerank", got.Name())

	_, err = r.Rerank("nonexistent")
	assert.Error(t, err)
}

func TestRouter_RegisterAndGetTTS(t *testing.T) {
	r := NewRouter()
	p := &mockTTSProvider{name: "test-tts"}
	r.RegisterTTS("test-tts", p, true)

	got, err := r.TTS("")
	require.NoError(t, err)
	assert.Equal(t, "test-tts", got.Name())
}

func TestRouter_RegisterAndGetSTT(t *testing.T) {
	r := NewRouter()
	p := &mockSTTProvider{name: "test-stt"}
	r.RegisterSTT("test-stt", p, true)

	got, err := r.STT("")
	require.NoError(t, err)
	assert.Equal(t, "test-stt", got.Name())
}

func TestRouter_RegisterAndGetImage(t *testing.T) {
	r := NewRouter()
	p := &mockImageProvider{name: "test-image"}
	r.RegisterImage("test-image", p, true)

	got, err := r.Image("")
	require.NoError(t, err)
	assert.Equal(t, "test-image", got.Name())
}

func TestRouter_RegisterAndGetVideo(t *testing.T) {
	r := NewRouter()
	p := &mockVideoProvider{name: "test-video"}
	r.RegisterVideo("test-video", p, true)

	got, err := r.Video("")
	require.NoError(t, err)
	assert.Equal(t, "test-video", got.Name())
}

func TestRouter_DefaultProviderAutoSet(t *testing.T) {
	r := NewRouter()
	p1 := &mockRerankProvider{name: "first"}
	p2 := &mockRerankProvider{name: "second"}

	// First registration auto-sets default
	r.RegisterRerank("first", p1, false)
	got, err := r.Rerank("")
	require.NoError(t, err)
	assert.Equal(t, "first", got.Name())

	// Second registration without isDefault doesn't change default
	r.RegisterRerank("second", p2, false)
	got, err = r.Rerank("")
	require.NoError(t, err)
	assert.Equal(t, "first", got.Name())

	// Explicit isDefault overrides
	r.RegisterRerank("second", p2, true)
	got, err = r.Rerank("")
	require.NoError(t, err)
	assert.Equal(t, "second", got.Name())
}

func TestRouter_HasCapability(t *testing.T) {
	r := NewRouter()
	assert.False(t, r.HasCapability(CapabilityEmbedding))
	assert.False(t, r.HasCapability(CapabilityRerank))

	r.RegisterEmbedding("e", &mockEmbeddingProvider{name: "e"}, true)
	assert.True(t, r.HasCapability(CapabilityEmbedding))
	assert.False(t, r.HasCapability(CapabilityRerank))

	// Unknown capability
	assert.False(t, r.HasCapability(Capability("unknown")))
}

func TestRouter_ListProviders(t *testing.T) {
	r := NewRouter()
	r.RegisterEmbedding("e1", &mockEmbeddingProvider{name: "e1"}, true)
	r.RegisterEmbedding("e2", &mockEmbeddingProvider{name: "e2"}, false)
	r.RegisterRerank("r1", &mockRerankProvider{name: "r1"}, true)

	providers := r.ListProviders()
	assert.Len(t, providers[CapabilityEmbedding], 2)
	assert.Len(t, providers[CapabilityRerank], 1)
	assert.Empty(t, providers[CapabilityTTS])
}

func TestRouter_Embed(t *testing.T) {
	r := NewRouter()
	r.RegisterEmbedding("e", &mockEmbeddingProvider{name: "e"}, true)

	resp, err := r.Embed(context.Background(), &embedding.EmbeddingRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "e", resp.Provider)
}

func TestRouter_RerankDocs(t *testing.T) {
	r := NewRouter()
	r.RegisterRerank("r", &mockRerankProvider{name: "r"}, true)

	resp, err := r.RerankDocs(context.Background(), &rerank.RerankRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "r", resp.Provider)
}

func TestRouter_Synthesize(t *testing.T) {
	r := NewRouter()
	r.RegisterTTS("tts", &mockTTSProvider{name: "tts"}, true)

	resp, err := r.Synthesize(context.Background(), &speech.TTSRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "tts", resp.Provider)
}

func TestRouter_Transcribe(t *testing.T) {
	r := NewRouter()
	r.RegisterSTT("stt", &mockSTTProvider{name: "stt"}, true)

	resp, err := r.Transcribe(context.Background(), &speech.STTRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "stt", resp.Provider)
}

func TestRouter_GenerateImage(t *testing.T) {
	r := NewRouter()
	r.RegisterImage("img", &mockImageProvider{name: "img"}, true)

	resp, err := r.GenerateImage(context.Background(), &image.GenerateRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "img", resp.Provider)
}

func TestRouter_GenerateVideo(t *testing.T) {
	r := NewRouter()
	r.RegisterVideo("vid", &mockVideoProvider{name: "vid"}, true)

	resp, err := r.GenerateVideo(context.Background(), &video.GenerateRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "vid", resp.Provider)
}

func TestRouter_NotFound_Errors(t *testing.T) {
	r := NewRouter()

	_, err := r.Embed(context.Background(), &embedding.EmbeddingRequest{}, "")
	assert.Error(t, err)

	_, err = r.RerankDocs(context.Background(), &rerank.RerankRequest{}, "")
	assert.Error(t, err)

	_, err = r.Synthesize(context.Background(), &speech.TTSRequest{}, "")
	assert.Error(t, err)

	_, err = r.Transcribe(context.Background(), &speech.STTRequest{}, "")
	assert.Error(t, err)

	_, err = r.GenerateImage(context.Background(), &image.GenerateRequest{}, "")
	assert.Error(t, err)

	_, err = r.GenerateVideo(context.Background(), &video.GenerateRequest{}, "")
	assert.Error(t, err)
}


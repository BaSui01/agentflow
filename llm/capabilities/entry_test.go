package capabilities

import (
	"context"
	"encoding/json"
	"testing"

	speech "github.com/BaSui01/agentflow/llm/capabilities/audio"
	"github.com/BaSui01/agentflow/llm/capabilities/avatar"
	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/moderation"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/music"
	"github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/llm/capabilities/threed"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
)

func TestNewEntry_UsesProvidedRouter(t *testing.T) {
	router := multimodal.NewRouter()
	entry := NewEntry(router)
	require.Same(t, router, entry.Router())
}

func TestNewEntry_CreatesDefaultRouter(t *testing.T) {
	entry := NewEntry(nil)
	require.NotNil(t, entry)
	require.NotNil(t, entry.Router())
}

func TestEntry_AllCapabilities(t *testing.T) {
	entry := NewEntry(multimodal.NewRouter())
	entry.Router().RegisterImage("img", &mockImageProvider{name: "img"}, true)
	entry.Router().RegisterVideo("vid", &mockVideoProvider{name: "vid"}, true)
	entry.Router().RegisterEmbedding("emb", &mockEmbeddingProvider{name: "emb"}, true)
	entry.Router().RegisterRerank("rerank", &mockRerankProvider{name: "rerank"}, true)
	entry.Router().RegisterTTS("tts", &mockTTSProvider{name: "tts"}, true)
	entry.Router().RegisterSTT("stt", &mockSTTProvider{name: "stt"}, true)
	entry.Router().RegisterMusic("music", &mockMusicProvider{name: "music"}, true)
	entry.Router().RegisterThreeD("threed", &mockThreeDProvider{name: "threed"}, true)
	entry.Router().RegisterModeration("mod", &mockModerationProvider{name: "mod"}, true)
	entry.RegisterAvatar("avatar", &mockAvatarProvider{name: "avatar"}, true)
	entry.SetToolExecutor(&mockToolExecutor{})

	imgResp, err := entry.GenerateImage(context.Background(), &image.GenerateRequest{Prompt: "p"}, "")
	require.NoError(t, err)
	require.Equal(t, "img", imgResp.Provider)

	vidResp, err := entry.GenerateVideo(context.Background(), &video.GenerateRequest{Prompt: "p"}, "")
	require.NoError(t, err)
	require.Equal(t, "vid", vidResp.Provider)

	embResp, err := entry.Embed(context.Background(), &embedding.EmbeddingRequest{Input: []string{"hello"}}, "")
	require.NoError(t, err)
	require.Equal(t, "emb", embResp.Provider)

	rerankResp, err := entry.RerankDocs(context.Background(), &rerank.RerankRequest{
		Query: "hello",
		Documents: []rerank.Document{
			{Text: "a"},
		},
	}, "")
	require.NoError(t, err)
	require.Equal(t, "rerank", rerankResp.Provider)

	ttsResp, err := entry.Synthesize(context.Background(), &speech.TTSRequest{Text: "hello"}, "")
	require.NoError(t, err)
	require.Equal(t, "tts", ttsResp.Provider)

	sttResp, err := entry.Transcribe(context.Background(), &speech.STTRequest{AudioURL: "https://example.com/a.mp3"}, "")
	require.NoError(t, err)
	require.Equal(t, "stt", sttResp.Provider)

	musicResp, err := entry.GenerateMusic(context.Background(), &music.GenerateRequest{Prompt: "song"}, "")
	require.NoError(t, err)
	require.Equal(t, "music", musicResp.Provider)

	threeDResp, err := entry.Generate3D(context.Background(), &threed.GenerateRequest{Prompt: "car"}, "")
	require.NoError(t, err)
	require.Equal(t, "threed", threeDResp.Provider)

	modResp, err := entry.Moderate(context.Background(), &moderation.ModerationRequest{Input: []string{"hi"}}, "")
	require.NoError(t, err)
	require.Equal(t, "mod", modResp.Provider)

	avatarResp, err := entry.GenerateAvatar(context.Background(), &avatar.GenerateRequest{Prompt: "avatar prompt"}, "")
	require.NoError(t, err)
	require.Equal(t, "avatar", avatarResp.Provider)

	toolResults, err := entry.ExecuteTools(context.Background(), []types.ToolCall{
		{
			ID:        "call_1",
			Name:      "mock_tool",
			Arguments: json.RawMessage(`{"q":"ok"}`),
		},
	})
	require.NoError(t, err)
	require.Len(t, toolResults, 1)
	require.Equal(t, "mock_tool", toolResults[0].Name)
}

func TestEntry_ResolveRerankProvider_ByChatProvider(t *testing.T) {
	entry := NewEntry(multimodal.NewRouter())
	entry.Router().RegisterRerank("qwen-rerank", &mockRerankProvider{name: "qwen-rerank"}, true)
	entry.Router().RegisterRerank("glm-rerank", &mockRerankProvider{name: "glm-rerank"}, false)
	require.NoError(t, entry.BindChatToRerank("qwen", "qwen-rerank"))
	require.NoError(t, entry.BindChatToRerank("GLM", "glm-rerank"))

	require.Equal(t, "qwen-rerank", entry.ResolveRerankProvider("qwen"))
	require.Equal(t, "glm-rerank", entry.ResolveRerankProvider("glm"))
	require.Equal(t, "", entry.ResolveRerankProvider("unknown"))
}

type mockImageProvider struct {
	name string
}

func (m *mockImageProvider) Generate(ctx context.Context, req *image.GenerateRequest) (*image.GenerateResponse, error) {
	return &image.GenerateResponse{Provider: m.name, Model: req.Model}, nil
}

func (m *mockImageProvider) Edit(ctx context.Context, req *image.EditRequest) (*image.GenerateResponse, error) {
	return &image.GenerateResponse{Provider: m.name}, nil
}

func (m *mockImageProvider) CreateVariation(ctx context.Context, req *image.VariationRequest) (*image.GenerateResponse, error) {
	return &image.GenerateResponse{Provider: m.name}, nil
}

func (m *mockImageProvider) Name() string { return m.name }

func (m *mockImageProvider) SupportedSizes() []string { return []string{"1024x1024"} }

type mockVideoProvider struct {
	name string
}

func (m *mockVideoProvider) Analyze(ctx context.Context, req *video.AnalyzeRequest) (*video.AnalyzeResponse, error) {
	return &video.AnalyzeResponse{Provider: m.name, Model: req.Model}, nil
}

func (m *mockVideoProvider) Generate(ctx context.Context, req *video.GenerateRequest) (*video.GenerateResponse, error) {
	return &video.GenerateResponse{Provider: m.name, Model: req.Model}, nil
}

func (m *mockVideoProvider) Name() string { return m.name }

func (m *mockVideoProvider) SupportedFormats() []video.VideoFormat {
	return []video.VideoFormat{video.VideoFormatMP4}
}

func (m *mockVideoProvider) SupportsGeneration() bool { return true }

type mockEmbeddingProvider struct {
	name string
}

func (m *mockEmbeddingProvider) Embed(ctx context.Context, req *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	return &embedding.EmbeddingResponse{
		Provider: m.name,
		Model:    req.Model,
		Embeddings: []embedding.EmbeddingData{
			{Index: 0, Embedding: []float64{0.1}},
		},
	}, nil
}

func (m *mockEmbeddingProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return []float64{0.1}, nil
}

func (m *mockEmbeddingProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return [][]float64{{0.1}}, nil
}

func (m *mockEmbeddingProvider) Name() string { return m.name }

func (m *mockEmbeddingProvider) Dimensions() int { return 1 }

func (m *mockEmbeddingProvider) MaxBatchSize() int { return 16 }

type mockRerankProvider struct {
	name string
}

func (m *mockRerankProvider) Rerank(ctx context.Context, req *rerank.RerankRequest) (*rerank.RerankResponse, error) {
	return &rerank.RerankResponse{
		Provider: m.name,
		Model:    req.Model,
		Results: []rerank.RerankResult{
			{Index: 0, RelevanceScore: 0.9},
		},
	}, nil
}

func (m *mockRerankProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]rerank.RerankResult, error) {
	return []rerank.RerankResult{{Index: 0, RelevanceScore: 0.9}}, nil
}

func (m *mockRerankProvider) Name() string { return m.name }

func (m *mockRerankProvider) MaxDocuments() int { return 64 }

type mockTTSProvider struct {
	name string
}

func (m *mockTTSProvider) Synthesize(ctx context.Context, req *speech.TTSRequest) (*speech.TTSResponse, error) {
	return &speech.TTSResponse{
		Provider: m.name,
		Model:    req.Model,
	}, nil
}

func (m *mockTTSProvider) SynthesizeToFile(ctx context.Context, req *speech.TTSRequest, filepath string) error {
	return nil
}

func (m *mockTTSProvider) ListVoices(ctx context.Context) ([]speech.Voice, error) {
	return []speech.Voice{{ID: "voice_1"}}, nil
}

func (m *mockTTSProvider) Name() string { return m.name }

type mockSTTProvider struct {
	name string
}

func (m *mockSTTProvider) Transcribe(ctx context.Context, req *speech.STTRequest) (*speech.STTResponse, error) {
	return &speech.STTResponse{
		Provider: m.name,
		Model:    req.Model,
		Text:     "ok",
	}, nil
}

func (m *mockSTTProvider) TranscribeFile(ctx context.Context, filepath string, opts *speech.STTRequest) (*speech.STTResponse, error) {
	return &speech.STTResponse{
		Provider: m.name,
		Model:    opts.Model,
		Text:     "ok",
	}, nil
}

func (m *mockSTTProvider) Name() string { return m.name }

func (m *mockSTTProvider) SupportedFormats() []string { return []string{"mp3"} }

type mockMusicProvider struct {
	name string
}

func (m *mockMusicProvider) Generate(ctx context.Context, req *music.GenerateRequest) (*music.GenerateResponse, error) {
	return &music.GenerateResponse{
		Provider: m.name,
		Model:    req.Model,
	}, nil
}

func (m *mockMusicProvider) Name() string { return m.name }

type mockThreeDProvider struct {
	name string
}

func (m *mockThreeDProvider) Generate(ctx context.Context, req *threed.GenerateRequest) (*threed.GenerateResponse, error) {
	return &threed.GenerateResponse{
		Provider: m.name,
		Model:    req.Model,
	}, nil
}

func (m *mockThreeDProvider) Name() string { return m.name }

type mockModerationProvider struct {
	name string
}

func (m *mockModerationProvider) Name() string { return m.name }

func (m *mockModerationProvider) Moderate(ctx context.Context, req *moderation.ModerationRequest) (*moderation.ModerationResponse, error) {
	return &moderation.ModerationResponse{
		Provider: m.name,
		Model:    req.Model,
		Results: []moderation.ModerationResult{
			{Flagged: false},
		},
	}, nil
}

type mockAvatarProvider struct {
	name string
}

func (m *mockAvatarProvider) Name() string { return m.name }

func (m *mockAvatarProvider) Generate(ctx context.Context, req *avatar.GenerateRequest) (*avatar.GenerateResponse, error) {
	return &avatar.GenerateResponse{
		Provider: m.name,
		Model:    req.Model,
		Assets: []avatar.AvatarData{
			{ID: "avatar_1"},
		},
	}, nil
}

type mockToolExecutor struct{}

func (m *mockToolExecutor) Execute(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	out := make([]types.ToolResult, 0, len(calls))
	for _, call := range calls {
		out = append(out, types.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Result:     json.RawMessage(`{"ok":true}`),
		})
	}
	return out
}

func (m *mockToolExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	return types.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Result:     json.RawMessage(`{"ok":true}`),
	}
}

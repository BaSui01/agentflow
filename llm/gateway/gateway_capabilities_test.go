package gateway

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/BaSui01/agentflow/llm/capabilities"
	speech "github.com/BaSui01/agentflow/llm/capabilities/audio"
	"github.com/BaSui01/agentflow/llm/capabilities/avatar"
	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/moderation"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/music"
	"github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/llm/capabilities/threed"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestService_Invoke_CapabilityBranches(t *testing.T) {
	svc := newCapabilityServiceForTest()
	ctx := context.Background()

	tests := []struct {
		name       string
		request    *llmcore.UnifiedRequest
		outputType any
	}{
		{
			name: "tools",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityTools,
				Payload: &ToolsInput{
					Calls: []types.ToolCall{
						{ID: "tool_1", Name: "web_search", Arguments: json.RawMessage(`{"query":"go"}`)},
					},
				},
			},
			outputType: []types.ToolResult{},
		},
		{
			name: "audio_tts",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityAudio,
				Payload: &AudioInput{
					Synthesize: &speech.TTSRequest{Text: "hello"},
				},
			},
			outputType: &speech.TTSResponse{},
		},
		{
			name: "audio_stt",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityAudio,
				Payload: &AudioInput{
					Transcribe: &speech.STTRequest{AudioURL: "https://example.com/audio.mp3"},
				},
			},
			outputType: &speech.STTResponse{},
		},
		{
			name: "embedding",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityEmbedding,
				Payload: &EmbeddingInput{
					Request: &embedding.EmbeddingRequest{Input: []string{"hello"}},
				},
			},
			outputType: &embedding.EmbeddingResponse{},
		},
		{
			name: "rerank",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityRerank,
				Payload: &RerankInput{
					Request: &rerank.RerankRequest{
						Query: "hello",
						Documents: []rerank.Document{
							{Text: "hello world"},
						},
					},
				},
			},
			outputType: &rerank.RerankResponse{},
		},
		{
			name: "moderation",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityModeration,
				Payload: &ModerationInput{
					Request: &moderation.ModerationRequest{Input: []string{"safe"}},
				},
			},
			outputType: &moderation.ModerationResponse{},
		},
		{
			name: "music",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityMusic,
				Payload: &MusicInput{
					Generate: &music.GenerateRequest{Prompt: "lofi beats"},
				},
			},
			outputType: &music.GenerateResponse{},
		},
		{
			name: "threed",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityThreeD,
				Payload: &ThreeDInput{
					Generate: &threed.GenerateRequest{Prompt: "futuristic chair"},
				},
			},
			outputType: &threed.GenerateResponse{},
		},
		{
			name: "avatar",
			request: &llmcore.UnifiedRequest{
				Capability: llmcore.CapabilityAvatar,
				Payload: &AvatarInput{
					Generate: &avatar.GenerateRequest{Prompt: "digital host"},
				},
			},
			outputType: &avatar.GenerateResponse{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := svc.Invoke(ctx, tt.request)
			require.NoError(t, err)
			require.NotNil(t, resp)
			require.IsType(t, tt.outputType, resp.Output)
		})
	}
}

func newCapabilityServiceForTest() *Service {
	router := multimodal.NewRouter()
	router.RegisterEmbedding("emb", &gatewayEmbeddingProvider{}, true)
	router.RegisterRerank("rerank", &gatewayRerankProvider{name: "rerank"}, true)
	router.RegisterRerank("qwen-rerank", &gatewayRerankProvider{name: "qwen-rerank"}, false)
	router.RegisterTTS("tts", &gatewayTTSProvider{}, true)
	router.RegisterSTT("stt", &gatewaySTTProvider{}, true)
	router.RegisterMusic("music", &gatewayMusicProvider{}, true)
	router.RegisterThreeD("threed", &gatewayThreeDProvider{}, true)
	router.RegisterModeration("mod", &gatewayModerationProvider{}, true)
	entry := capabilities.NewEntry(router)
	if err := entry.BindChatToRerank("qwen", "qwen-rerank"); err != nil {
		panic(err)
	}
	entry.RegisterAvatar("avatar", &gatewayAvatarProvider{}, true)
	entry.SetToolExecutor(&gatewayToolExecutor{})
	return New(Config{Capabilities: entry, Logger: zap.NewNop()})
}

type gatewayEmbeddingProvider struct{}

func (p *gatewayEmbeddingProvider) Embed(ctx context.Context, req *embedding.EmbeddingRequest) (*embedding.EmbeddingResponse, error) {
	return &embedding.EmbeddingResponse{
		Provider: "emb",
		Model:    req.Model,
		Embeddings: []embedding.EmbeddingData{
			{Index: 0, Embedding: []float64{0.1}},
		},
		Usage: embedding.EmbeddingUsage{
			PromptTokens: 8,
			TotalTokens:  8,
			Cost:         0.0001,
		},
	}, nil
}

func (p *gatewayEmbeddingProvider) EmbedQuery(ctx context.Context, query string) ([]float64, error) {
	return []float64{0.1}, nil
}

func (p *gatewayEmbeddingProvider) EmbedDocuments(ctx context.Context, documents []string) ([][]float64, error) {
	return [][]float64{{0.1}}, nil
}

func (p *gatewayEmbeddingProvider) Name() string { return "emb" }

func (p *gatewayEmbeddingProvider) Dimensions() int { return 1 }

func (p *gatewayEmbeddingProvider) MaxBatchSize() int { return 32 }

type gatewayRerankProvider struct {
	name string
}

func (p *gatewayRerankProvider) Rerank(ctx context.Context, req *rerank.RerankRequest) (*rerank.RerankResponse, error) {
	return &rerank.RerankResponse{
		Provider: p.name,
		Model:    req.Model,
		Results: []rerank.RerankResult{
			{Index: 0, RelevanceScore: 0.9},
		},
		Usage: rerank.RerankUsage{
			TotalTokens: 11,
			Cost:        0.0002,
		},
	}, nil
}

func (p *gatewayRerankProvider) RerankSimple(ctx context.Context, query string, documents []string, topN int) ([]rerank.RerankResult, error) {
	return []rerank.RerankResult{{Index: 0, RelevanceScore: 0.9}}, nil
}

func (p *gatewayRerankProvider) Name() string { return p.name }

func (p *gatewayRerankProvider) MaxDocuments() int { return 128 }

func TestService_Invoke_RerankBindingByChatProvider(t *testing.T) {
	svc := newCapabilityServiceForTest()
	resp, err := svc.Invoke(context.Background(), &llmcore.UnifiedRequest{
		Capability: llmcore.CapabilityRerank,
		Hints: llmcore.CapabilityHints{
			ChatProvider: "qwen",
		},
		Payload: &RerankInput{
			Request: &rerank.RerankRequest{
				Query: "hello",
				Documents: []rerank.Document{
					{Text: "hello world"},
				},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	rerankResp, ok := resp.Output.(*rerank.RerankResponse)
	require.True(t, ok)
	require.Equal(t, "qwen-rerank", rerankResp.Provider)
}

type gatewayTTSProvider struct{}

func (p *gatewayTTSProvider) Synthesize(ctx context.Context, req *speech.TTSRequest) (*speech.TTSResponse, error) {
	return &speech.TTSResponse{Provider: "tts", Model: req.Model}, nil
}

func (p *gatewayTTSProvider) SynthesizeToFile(ctx context.Context, req *speech.TTSRequest, filepath string) error {
	return nil
}

func (p *gatewayTTSProvider) ListVoices(ctx context.Context) ([]speech.Voice, error) {
	return []speech.Voice{{ID: "voice_1"}}, nil
}

func (p *gatewayTTSProvider) Name() string { return "tts" }

type gatewaySTTProvider struct{}

func (p *gatewaySTTProvider) Transcribe(ctx context.Context, req *speech.STTRequest) (*speech.STTResponse, error) {
	return &speech.STTResponse{Provider: "stt", Model: req.Model, Text: "ok"}, nil
}

func (p *gatewaySTTProvider) TranscribeFile(ctx context.Context, filepath string, opts *speech.STTRequest) (*speech.STTResponse, error) {
	return &speech.STTResponse{Provider: "stt", Model: opts.Model, Text: "ok"}, nil
}

func (p *gatewaySTTProvider) Name() string { return "stt" }

func (p *gatewaySTTProvider) SupportedFormats() []string { return []string{"mp3"} }

type gatewayModerationProvider struct{}

func (p *gatewayModerationProvider) Name() string { return "mod" }

func (p *gatewayModerationProvider) Moderate(ctx context.Context, req *moderation.ModerationRequest) (*moderation.ModerationResponse, error) {
	return &moderation.ModerationResponse{
		Provider: "mod",
		Model:    req.Model,
		Results: []moderation.ModerationResult{
			{Flagged: false},
		},
	}, nil
}

type gatewayMusicProvider struct{}

func (p *gatewayMusicProvider) Name() string { return "music" }

func (p *gatewayMusicProvider) Generate(ctx context.Context, req *music.GenerateRequest) (*music.GenerateResponse, error) {
	return &music.GenerateResponse{
		Provider: "music",
		Model:    req.Model,
		Tracks: []music.MusicData{
			{ID: "track_1"},
		},
		Usage: music.MusicUsage{
			TracksGenerated: 1,
			Credits:         0.3,
		},
	}, nil
}

type gatewayThreeDProvider struct{}

func (p *gatewayThreeDProvider) Name() string { return "threed" }

func (p *gatewayThreeDProvider) Generate(ctx context.Context, req *threed.GenerateRequest) (*threed.GenerateResponse, error) {
	return &threed.GenerateResponse{
		Provider: "threed",
		Model:    req.Model,
		Models: []threed.ModelData{
			{ID: "model_1"},
		},
		Usage: threed.ThreeDUsage{
			ModelsGenerated: 1,
			Credits:         1.2,
		},
	}, nil
}

type gatewayToolExecutor struct{}

func (e *gatewayToolExecutor) Execute(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	results := make([]types.ToolResult, 0, len(calls))
	for _, call := range calls {
		results = append(results, types.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Result:     json.RawMessage(`{"ok":true}`),
		})
	}
	return results
}

func (e *gatewayToolExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	return types.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Result:     json.RawMessage(`{"ok":true}`),
	}
}

type gatewayAvatarProvider struct{}

func (p *gatewayAvatarProvider) Name() string { return "avatar" }

func (p *gatewayAvatarProvider) Generate(ctx context.Context, req *avatar.GenerateRequest) (*avatar.GenerateResponse, error) {
	return &avatar.GenerateResponse{
		Provider: "avatar",
		Model:    req.Model,
		Assets: []avatar.AvatarData{
			{ID: "asset_1"},
		},
		Usage: avatar.AvatarUsage{
			AvatarsGenerated: 1,
			Credits:          2.5,
		},
	}, nil
}

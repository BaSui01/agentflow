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

// ---------------------------------------------------------------------------
// nil Entry — every method must be safe to call on a nil *Entry
// ---------------------------------------------------------------------------

func TestNilEntry_Router(t *testing.T) {
	var e *Entry
	require.Nil(t, e.Router())
}

func TestNilEntry_SetToolExecutor(t *testing.T) {
	var e *Entry
	e.SetToolExecutor(&mockToolExecutor{}) // should not panic
}

func TestNilEntry_ToolExecutor(t *testing.T) {
	var e *Entry
	require.Nil(t, e.ToolExecutor())
}

func TestNilEntry_RegisterAvatar(t *testing.T) {
	var e *Entry
	e.RegisterAvatar("x", &mockAvatarProvider{name: "x"}, true) // no panic
}

func TestNilEntry_Avatar(t *testing.T) {
	var e *Entry
	_, err := e.Avatar("any")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not configured")
}

func TestNilEntry_Embedding(t *testing.T) {
	var e *Entry
	_, err := e.Embedding("")
	require.Error(t, err)
}

func TestNilEntry_Rerank(t *testing.T) {
	var e *Entry
	_, err := e.Rerank("")
	require.Error(t, err)
}

func TestNilEntry_TTS(t *testing.T) {
	var e *Entry
	_, err := e.TTS("")
	require.Error(t, err)
}

func TestNilEntry_STT(t *testing.T) {
	var e *Entry
	_, err := e.STT("")
	require.Error(t, err)
}

func TestNilEntry_Image(t *testing.T) {
	var e *Entry
	_, err := e.Image("")
	require.Error(t, err)
}

func TestNilEntry_Video(t *testing.T) {
	var e *Entry
	_, err := e.Video("")
	require.Error(t, err)
}

func TestNilEntry_Music(t *testing.T) {
	var e *Entry
	_, err := e.Music("")
	require.Error(t, err)
}

func TestNilEntry_ThreeD(t *testing.T) {
	var e *Entry
	_, err := e.ThreeD("")
	require.Error(t, err)
}

func TestNilEntry_Moderation(t *testing.T) {
	var e *Entry
	_, err := e.Moderation("")
	require.Error(t, err)
}

func TestNilEntry_Embed(t *testing.T) {
	var e *Entry
	_, err := e.Embed(context.Background(), &embedding.EmbeddingRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_RerankDocs(t *testing.T) {
	var e *Entry
	_, err := e.RerankDocs(context.Background(), &rerank.RerankRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_Synthesize(t *testing.T) {
	var e *Entry
	_, err := e.Synthesize(context.Background(), &speech.TTSRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_Transcribe(t *testing.T) {
	var e *Entry
	_, err := e.Transcribe(context.Background(), &speech.STTRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_GenerateImage(t *testing.T) {
	var e *Entry
	_, err := e.GenerateImage(context.Background(), &image.GenerateRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_GenerateVideo(t *testing.T) {
	var e *Entry
	_, err := e.GenerateVideo(context.Background(), &video.GenerateRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_GenerateMusic(t *testing.T) {
	var e *Entry
	_, err := e.GenerateMusic(context.Background(), &music.GenerateRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_Generate3D(t *testing.T) {
	var e *Entry
	_, err := e.Generate3D(context.Background(), &threed.GenerateRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_Moderate(t *testing.T) {
	var e *Entry
	_, err := e.Moderate(context.Background(), &moderation.ModerationRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_GenerateAvatar(t *testing.T) {
	var e *Entry
	_, err := e.GenerateAvatar(context.Background(), &avatar.GenerateRequest{}, "")
	require.Error(t, err)
}

func TestNilEntry_ExecuteTools(t *testing.T) {
	var e *Entry
	_, err := e.ExecuteTools(context.Background(), nil)
	require.Error(t, err)
}

func TestNilEntry_ExecuteTool(t *testing.T) {
	var e *Entry
	_, err := e.ExecuteTool(context.Background(), types.ToolCall{})
	require.Error(t, err)
}

func TestNilEntry_ResolveRerankProvider(t *testing.T) {
	var e *Entry
	require.Equal(t, "", e.ResolveRerankProvider("anything"))
}

func TestNilEntry_BindChatToRerank(t *testing.T) {
	var e *Entry
	err := e.BindChatToRerank("chat", "rerank")
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Avatar registration & lookup
// ---------------------------------------------------------------------------

func TestRegisterAvatar_IgnoresEmptyName(t *testing.T) {
	entry := NewEntry(nil)
	entry.RegisterAvatar("", &mockAvatarProvider{name: "x"}, true)
	_, err := entry.Avatar("")
	require.Error(t, err, "empty-name registration should be ignored")
}

func TestRegisterAvatar_IgnoresNilProvider(t *testing.T) {
	entry := NewEntry(nil)
	entry.RegisterAvatar("x", nil, true)
	_, err := entry.Avatar("x")
	require.Error(t, err)
}

func TestAvatar_DefaultProvider(t *testing.T) {
	entry := NewEntry(nil)
	entry.RegisterAvatar("first", &mockAvatarProvider{name: "first"}, false)
	// first registered becomes default when no explicit default
	p, err := entry.Avatar("")
	require.NoError(t, err)
	require.Equal(t, "first", p.Name())

	// register second as explicit default
	entry.RegisterAvatar("second", &mockAvatarProvider{name: "second"}, true)
	p, err = entry.Avatar("")
	require.NoError(t, err)
	require.Equal(t, "second", p.Name())
}

func TestAvatar_NotFound(t *testing.T) {
	entry := NewEntry(nil)
	_, err := entry.Avatar("nonexistent")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
}

// ---------------------------------------------------------------------------
// ToolExecutor get/set
// ---------------------------------------------------------------------------

func TestToolExecutor_SetAndGet(t *testing.T) {
	entry := NewEntry(nil)
	require.Nil(t, entry.ToolExecutor())
	exec := &mockToolExecutor{}
	entry.SetToolExecutor(exec)
	require.Same(t, exec, entry.ToolExecutor())
}

// ---------------------------------------------------------------------------
// ExecuteTool (single tool call)
// ---------------------------------------------------------------------------

func TestExecuteTool_Success(t *testing.T) {
	entry := NewEntry(nil)
	entry.SetToolExecutor(&mockToolExecutor{})
	result, err := entry.ExecuteTool(context.Background(), types.ToolCall{
		ID:        "c1",
		Name:      "test_tool",
		Arguments: json.RawMessage(`{}`),
	})
	require.NoError(t, err)
	require.Equal(t, "test_tool", result.Name)
	require.Equal(t, "c1", result.ToolCallID)
}

func TestExecuteTool_NoExecutor(t *testing.T) {
	entry := NewEntry(nil)
	_, err := entry.ExecuteTool(context.Background(), types.ToolCall{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not configured")
}

func TestExecuteTools_NoExecutor(t *testing.T) {
	entry := NewEntry(nil)
	_, err := entry.ExecuteTools(context.Background(), []types.ToolCall{{ID: "c1"}})
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// BindChatToRerank error paths
// ---------------------------------------------------------------------------

func TestBindChatToRerank_EmptyNames(t *testing.T) {
	entry := NewEntry(multimodal.NewRouter())
	err := entry.BindChatToRerank("", "rerank")
	require.Error(t, err)
	require.Contains(t, err.Error(), "required")

	err = entry.BindChatToRerank("chat", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "required")
}

func TestBindChatToRerank_RerankNotRegistered(t *testing.T) {
	entry := NewEntry(multimodal.NewRouter())
	err := entry.BindChatToRerank("chat", "nonexistent")
	require.Error(t, err)
}

func TestResolveRerankProvider_EmptyChat(t *testing.T) {
	entry := NewEntry(nil)
	require.Equal(t, "", entry.ResolveRerankProvider(""))
}

func TestResolveRerankProvider_SpacesOnly(t *testing.T) {
	entry := NewEntry(nil)
	require.Equal(t, "", entry.ResolveRerankProvider("   "))
}

// ---------------------------------------------------------------------------
// Provider getter methods on entry with nil router
// ---------------------------------------------------------------------------

func TestEntry_NilRouter_ProviderGetters(t *testing.T) {
	// Entry with router explicitly set to nil after construction
	entry := &Entry{}
	tests := []struct {
		name string
		fn   func() error
	}{
		{"Embedding", func() error { _, e := entry.Embedding(""); return e }},
		{"Rerank", func() error { _, e := entry.Rerank(""); return e }},
		{"TTS", func() error { _, e := entry.TTS(""); return e }},
		{"STT", func() error { _, e := entry.STT(""); return e }},
		{"Image", func() error { _, e := entry.Image(""); return e }},
		{"Video", func() error { _, e := entry.Video(""); return e }},
		{"Music", func() error { _, e := entry.Music(""); return e }},
		{"ThreeD", func() error { _, e := entry.ThreeD(""); return e }},
		{"Moderation", func() error { _, e := entry.Moderation(""); return e }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			require.Error(t, err)
			require.Contains(t, err.Error(), "not configured")
		})
	}
}

// ---------------------------------------------------------------------------
// Capability call methods on entry with nil router
// ---------------------------------------------------------------------------

func TestEntry_NilRouter_CapabilityCalls(t *testing.T) {
	entry := &Entry{}
	ctx := context.Background()

	_, err := entry.Embed(ctx, &embedding.EmbeddingRequest{}, "")
	require.Error(t, err)

	_, err = entry.RerankDocs(ctx, &rerank.RerankRequest{}, "")
	require.Error(t, err)

	_, err = entry.Synthesize(ctx, &speech.TTSRequest{}, "")
	require.Error(t, err)

	_, err = entry.Transcribe(ctx, &speech.STTRequest{}, "")
	require.Error(t, err)

	_, err = entry.GenerateImage(ctx, &image.GenerateRequest{}, "")
	require.Error(t, err)

	_, err = entry.GenerateVideo(ctx, &video.GenerateRequest{}, "")
	require.Error(t, err)

	_, err = entry.GenerateMusic(ctx, &music.GenerateRequest{}, "")
	require.Error(t, err)

	_, err = entry.Generate3D(ctx, &threed.GenerateRequest{}, "")
	require.Error(t, err)

	_, err = entry.Moderate(ctx, &moderation.ModerationRequest{}, "")
	require.Error(t, err)
}

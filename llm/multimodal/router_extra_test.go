package multimodal

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/llm/moderation"
	"github.com/BaSui01/agentflow/llm/music"
	"github.com/BaSui01/agentflow/llm/threed"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock providers for uncovered Router methods ---

type mockMusicProvider struct{ name string }

func (m *mockMusicProvider) Name() string { return m.name }
func (m *mockMusicProvider) Generate(_ context.Context, _ *music.GenerateRequest) (*music.GenerateResponse, error) {
	return &music.GenerateResponse{Provider: m.name}, nil
}

type mockThreeDProvider struct{ name string }

func (m *mockThreeDProvider) Name() string { return m.name }
func (m *mockThreeDProvider) Generate(_ context.Context, _ *threed.GenerateRequest) (*threed.GenerateResponse, error) {
	return &threed.GenerateResponse{Provider: m.name}, nil
}

type mockModerationProvider struct{ name string }

func (m *mockModerationProvider) Name() string { return m.name }
func (m *mockModerationProvider) Moderate(_ context.Context, _ *moderation.ModerationRequest) (*moderation.ModerationResponse, error) {
	return &moderation.ModerationResponse{Provider: m.name}, nil
}

// --- Tests ---

func TestRouter_RegisterAndGetMusic(t *testing.T) {
	r := NewRouter()
	p := &mockMusicProvider{name: "test-music"}
	r.RegisterMusic("test-music", p, true)

	got, err := r.Music("")
	require.NoError(t, err)
	assert.Equal(t, "test-music", got.Name())

	got, err = r.Music("test-music")
	require.NoError(t, err)
	assert.Equal(t, "test-music", got.Name())

	_, err = r.Music("nonexistent")
	assert.Error(t, err)
}
func TestRouter_RegisterAndGetThreeD(t *testing.T) {
	r := NewRouter()
	p := &mockThreeDProvider{name: "test-3d"}
	r.RegisterThreeD("test-3d", p, true)

	got, err := r.ThreeD("")
	require.NoError(t, err)
	assert.Equal(t, "test-3d", got.Name())

	_, err = r.ThreeD("nonexistent")
	assert.Error(t, err)
}

func TestRouter_RegisterAndGetModeration(t *testing.T) {
	r := NewRouter()
	p := &mockModerationProvider{name: "test-mod"}
	r.RegisterModeration("test-mod", p, true)

	got, err := r.Moderation("")
	require.NoError(t, err)
	assert.Equal(t, "test-mod", got.Name())

	_, err = r.Moderation("nonexistent")
	assert.Error(t, err)
}

func TestRouter_GenerateMusic(t *testing.T) {
	r := NewRouter()
	p := &mockMusicProvider{name: "test-music"}
	r.RegisterMusic("test-music", p, true)

	resp, err := r.GenerateMusic(context.Background(), &music.GenerateRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "test-music", resp.Provider)
}

func TestRouter_Generate3D(t *testing.T) {
	r := NewRouter()
	p := &mockThreeDProvider{name: "test-3d"}
	r.RegisterThreeD("test-3d", p, true)

	resp, err := r.Generate3D(context.Background(), &threed.GenerateRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "test-3d", resp.Provider)
}

func TestRouter_Moderate(t *testing.T) {
	r := NewRouter()
	p := &mockModerationProvider{name: "test-mod"}
	r.RegisterModeration("test-mod", p, true)

	resp, err := r.Moderate(context.Background(), &moderation.ModerationRequest{}, "")
	require.NoError(t, err)
	assert.Equal(t, "test-mod", resp.Provider)
}

func TestRouter_HasCapability_AllTypes(t *testing.T) {
	r := NewRouter()

	// Initially no capabilities
	assert.False(t, r.HasCapability(CapabilityMusic))
	assert.False(t, r.HasCapability(CapabilityThreeD))
	assert.False(t, r.HasCapability(CapabilityModeration))

	// Register providers
	r.RegisterMusic("m", &mockMusicProvider{name: "m"}, true)
	r.RegisterThreeD("3d", &mockThreeDProvider{name: "3d"}, true)
	r.RegisterModeration("mod", &mockModerationProvider{name: "mod"}, true)

	assert.True(t, r.HasCapability(CapabilityMusic))
	assert.True(t, r.HasCapability(CapabilityThreeD))
	assert.True(t, r.HasCapability(CapabilityModeration))
}

func TestRouter_ListProviders_AllTypes(t *testing.T) {
	r := NewRouter()
	r.RegisterMusic("m1", &mockMusicProvider{name: "m1"}, true)
	r.RegisterThreeD("3d1", &mockThreeDProvider{name: "3d1"}, true)
	r.RegisterModeration("mod1", &mockModerationProvider{name: "mod1"}, true)

	providers := r.ListProviders()
	assert.Contains(t, providers, CapabilityMusic)
	assert.Contains(t, providers, CapabilityThreeD)
	assert.Contains(t, providers, CapabilityModeration)
}


package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock plugin with metadata ---

type mockMetadataPlugin struct {
	mockPlugin
	meta PluginMetadata
}

func newMockMetadataPlugin(name, version string, tags ...string) *mockMetadataPlugin {
	return &mockMetadataPlugin{
		mockPlugin: mockPlugin{name: name, version: version},
		meta: PluginMetadata{
			Name:        name,
			Version:     version,
			Description: name + " plugin",
			Tags:        tags,
		},
	}
}

func (m *mockMetadataPlugin) Metadata() PluginMetadata { return m.meta }

// --- ExtractMetadata ---

func TestExtractMetadata(t *testing.T) {
	tests := []struct {
		name     string
		plugin   Plugin
		wantName string
		wantDesc string
		wantTags []string
	}{
		{
			name:     "plugin without MetadataProvider",
			plugin:   newMockPlugin("basic", "1.0"),
			wantName: "basic",
		},
		{
			name:     "plugin with MetadataProvider",
			plugin:   newMockMetadataPlugin("rich", "2.0", "llm", "rag"),
			wantName: "rich",
			wantDesc: "rich plugin",
			wantTags: []string{"llm", "rag"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ExtractMetadata(tt.plugin)
			assert.Equal(t, tt.wantName, m.Name)
			assert.Equal(t, tt.wantDesc, m.Description)
			if tt.wantTags != nil {
				assert.Equal(t, tt.wantTags, m.Tags)
			}
		})
	}
}

// --- PluginManager ---

func newTestManager(t *testing.T) *PluginManager {
	t.Helper()
	return NewPluginManager(NewInMemoryPluginRegistry(nil), nil)
}

func TestNewPluginManager(t *testing.T) {
	m := NewPluginManager(NewInMemoryPluginRegistry(nil), nil)
	require.NotNil(t, m)
	require.NotNil(t, m.Registry())
}

func TestPluginManager_Register(t *testing.T) {
	tests := []struct {
		name    string
		plugin  Plugin
		wantErr bool
	}{
		{
			name:   "basic plugin uses derived metadata",
			plugin: newMockPlugin("basic", "1.0"),
		},
		{
			name:   "metadata provider plugin uses own metadata",
			plugin: newMockMetadataPlugin("rich", "2.0", "tag1"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestManager(t)
			err := m.Register(tt.plugin)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			info, ok := m.Registry().Get(tt.plugin.Name())
			require.True(t, ok)
			assert.Equal(t, PluginStateRegistered, info.State)
		})
	}
}

func TestPluginManager_Register_Duplicate(t *testing.T) {
	m := newTestManager(t)
	require.NoError(t, m.Register(newMockPlugin("dup", "1.0")))
	err := m.Register(newMockPlugin("dup", "1.0"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPluginAlreadyRegistered)
}

func TestPluginManager_RegisterWithMetadata(t *testing.T) {
	m := newTestManager(t)
	p := newMockPlugin("custom", "1.0")
	customMeta := PluginMetadata{
		Name:        "custom",
		Version:     "1.0",
		Description: "overridden description",
		Tags:        []string{"special"},
	}
	require.NoError(t, m.RegisterWithMetadata(p, customMeta))

	info, ok := m.Registry().Get("custom")
	require.True(t, ok)
	assert.Equal(t, "overridden description", info.Metadata.Description)
	assert.Equal(t, []string{"special"}, info.Metadata.Tags)
}

func TestPluginManager_InitAll(t *testing.T) {
	m := newTestManager(t)
	p1 := newMockPlugin("a", "1.0")
	p2 := newMockPlugin("b", "1.0")
	require.NoError(t, m.Register(p1))
	require.NoError(t, m.Register(p2))

	require.NoError(t, m.InitAll(context.Background()))

	info1, _ := m.Registry().Get("a")
	assert.Equal(t, PluginStateInitialized, info1.State)
	info2, _ := m.Registry().Get("b")
	assert.Equal(t, PluginStateInitialized, info2.State)
}

func TestPluginManager_InitAll_WithError(t *testing.T) {
	m := newTestManager(t)
	p := newMockPlugin("fail", "1.0")
	p.initErr = errors.New("init-boom")
	require.NoError(t, m.Register(p))

	err := m.InitAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "init-boom")
}

func TestPluginManager_ShutdownAll(t *testing.T) {
	m := newTestManager(t)
	p1 := newMockPlugin("a", "1.0")
	p2 := newMockPlugin("b", "1.0")
	require.NoError(t, m.Register(p1))
	require.NoError(t, m.Register(p2))
	require.NoError(t, m.InitAll(context.Background()))

	require.NoError(t, m.ShutdownAll(context.Background()))

	info1, _ := m.Registry().Get("a")
	assert.Equal(t, PluginStateShutdown, info1.State)
	info2, _ := m.Registry().Get("b")
	assert.Equal(t, PluginStateShutdown, info2.State)
}

func TestPluginManager_ShutdownAll_WithError(t *testing.T) {
	m := newTestManager(t)
	p := newMockPlugin("fail", "1.0")
	p.shutdownErr = errors.New("shutdown-boom")
	require.NoError(t, m.Register(p))
	require.NoError(t, m.InitAll(context.Background()))

	err := m.ShutdownAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown-boom")
}

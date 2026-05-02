package bootstrap

import (
	"github.com/BaSui01/agentflow/config"
	"os"
	"path/filepath"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestBuildModelCatalog_EmptyPathUsesDefaultSnapshot(t *testing.T) {
	catalog, err := BuildModelCatalog("  ")
	require.NoError(t, err)
	require.NotNil(t, catalog)

	model, ok := catalog.Lookup("openai", "gpt-5.4")
	require.True(t, ok)
	assert.Equal(t, types.DefaultModelCatalogVerifiedAt, model.VerifiedAt)
}

func TestBuildModelCatalog_LoadsExternalSnapshot(t *testing.T) {
	tmpDir := t.TempDir()
	catalogPath := filepath.Join(tmpDir, "models.json")
	payload := `{
        "verified_at": "2026-05-02",
        "source": "test",
        "models": [
            {"provider": "openai", "id": "gpt-test", "aliases": ["default"], "capabilities": ["text_input"]}
        ]
    }`
	require.NoError(t, os.WriteFile(catalogPath, []byte(payload), 0644))

	catalog, err := BuildModelCatalog(catalogPath)
	require.NoError(t, err)
	require.NotNil(t, catalog)

	model, ok := catalog.Lookup("openai", "default")
	require.True(t, ok)
	assert.Equal(t, "gpt-test", model.ID)
	assert.True(t, model.Supports(types.ModelCapabilityTextInput))
}

func TestBuildModelCatalog_ReturnsLoadErrors(t *testing.T) {
	_, err := BuildModelCatalog(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build model catalog")
}

func TestBuildServeHandlerSet_ReturnsModelCatalogLoadError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.LLM.ModelCatalogPath = filepath.Join(t.TempDir(), "missing-models.json")

	set, err := BuildServeHandlerSet(ServeHandlerSetBuildInput{
		Cfg:    cfg,
		Logger: zap.NewNop(),
	})
	require.Error(t, err)
	require.Nil(t, set)
	assert.Contains(t, err.Error(), "build model catalog")
}

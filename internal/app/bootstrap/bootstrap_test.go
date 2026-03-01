package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BaSui01/agentflow/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadAndValidateConfig_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid.yaml")
	invalid := `
server:
  http_port: 70000
agent:
  name: "test-agent"
  model: "gpt-4o-mini"
  max_iterations: 3
  temperature: 0.5
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(invalid), 0o600))

	cfg, err := LoadAndValidateConfig(cfgPath)
	require.Error(t, err)
	require.Nil(t, cfg)
}

func TestNewLogger_FallbackOnInvalidOutputPath(t *testing.T) {
	logger := NewLogger(config.LogConfig{
		Level:       "info",
		Format:      "json",
		OutputPaths: []string{":://invalid-output-path"},
	})
	require.NotNil(t, logger)
	require.IsType(t, &zap.Logger{}, logger)
}

func TestOpenDatabase_UnsupportedDriver(t *testing.T) {
	logger := zap.NewNop()
	db, err := OpenDatabase(config.DatabaseConfig{
		Driver: "sqlite",
	}, logger)
	require.Error(t, err)
	require.Nil(t, db)
	require.Contains(t, err.Error(), "unsupported database driver")
}

package bootstrap

import (
	"fmt"
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
		Driver: "oracle",
	}, logger)
	require.Error(t, err)
	require.Nil(t, db)
	require.Contains(t, err.Error(), "unsupported database driver")
}

func TestInitializeServeRuntime_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid.yaml")
	invalid := `
server:
  http_port: 70000
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(invalid), 0o600))

	runtime, err := InitializeServeRuntime(cfgPath)
	require.Error(t, err)
	require.Nil(t, runtime)
}

func TestInitializeServeRuntime_Success(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "agentflow_test.sqlite")
	cfgPath := filepath.Join(dir, "valid.yaml")
	valid := fmt.Sprintf(`
server:
  http_port: 8088
agent:
  name: "test-agent"
  model: "gpt-4o-mini"
  max_iterations: 3
  temperature: 0.5
database:
  driver: "sqlite"
  name: %q
telemetry:
  enabled: false
`, dbPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(valid), 0o600))

	runtime, err := InitializeServeRuntime(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	require.NotNil(t, runtime.Config)
	require.NotNil(t, runtime.Logger)
	require.NotNil(t, runtime.Telemetry)
	if runtime.DB != nil {
		sqlDB, sqlErr := runtime.DB.DB()
		require.NoError(t, sqlErr)
		require.NoError(t, sqlDB.Close())
	}
}

package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

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

func TestLoadAndValidateConfig_ValidConfig(t *testing.T) {
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
  max_open_conns: 13
  max_idle_conns: 7
  conn_max_lifetime: 2m
telemetry:
  enabled: false
`, dbPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(valid), 0o600))

	cfg, err := LoadAndValidateConfig(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	require.Equal(t, 13, cfg.Database.MaxOpenConns)
	require.Equal(t, 7, cfg.Database.MaxIdleConns)
	require.Equal(t, 2*time.Minute, cfg.Database.ConnMaxLifetime)
}

func TestLoadAndValidateConfig_RejectsProductionAllowNoAuth(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid-auth.yaml")
	invalid := `
server:
  environment: production
  allow_no_auth: true
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
	require.Contains(t, err.Error(), "server.allow_no_auth cannot be true when server.environment=production")
}

type fakePoolConfigurer struct {
	maxOpenConns    int
	maxIdleConns    int
	connMaxLifetime time.Duration
}

func (f *fakePoolConfigurer) SetMaxOpenConns(v int) {
	f.maxOpenConns = v
}

func (f *fakePoolConfigurer) SetMaxIdleConns(v int) {
	f.maxIdleConns = v
}

func (f *fakePoolConfigurer) SetConnMaxLifetime(v time.Duration) {
	f.connMaxLifetime = v
}

func TestApplyDatabasePoolConfig_AppliesConfiguredValues(t *testing.T) {
	configurer := &fakePoolConfigurer{}
	applyDatabasePoolConfig(configurer, config.DatabaseConfig{
		MaxOpenConns:    11,
		MaxIdleConns:    5,
		ConnMaxLifetime: 3 * time.Minute,
	})

	require.Equal(t, 11, configurer.maxOpenConns)
	require.Equal(t, 5, configurer.maxIdleConns)
	require.Equal(t, 3*time.Minute, configurer.connMaxLifetime)
}

func TestInitializeServeRuntime_RequiresReachableDatabase(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "invalid-db.yaml")
	unreachableDBPath := filepath.Join(dir, "missing", "agentflow.sqlite")
	invalid := fmt.Sprintf(`
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
`, unreachableDBPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(invalid), 0o600))

	runtime, err := InitializeServeRuntime(cfgPath)
	require.Error(t, err)
	require.Nil(t, runtime)
	require.ErrorContains(t, err, "database is required for serve startup")
}

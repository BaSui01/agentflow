// =============================================================================
// 🧪 AgentFlow 配置加载器测试
// =============================================================================
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 🎯 默认配置测试
// =============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// 验证服务器默认值
	assert.Equal(t, 8080, cfg.Server.HTTPPort)
	assert.Equal(t, 9090, cfg.Server.GRPCPort)
	assert.Equal(t, 9091, cfg.Server.MetricsPort)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)

	// 验证 Agent 默认值
	assert.Equal(t, "default-agent", cfg.Agent.Name)
	assert.Equal(t, "gpt-4", cfg.Agent.Model)
	assert.Equal(t, 10, cfg.Agent.MaxIterations)
	assert.Equal(t, 0.7, cfg.Agent.Temperature)
	assert.True(t, cfg.Agent.StreamEnabled)

	// 验证 Memory 默认值
	assert.True(t, cfg.Agent.Memory.Enabled)
	assert.Equal(t, "buffer", cfg.Agent.Memory.Type)
	assert.Equal(t, 100, cfg.Agent.Memory.MaxMessages)

	// 验证 Redis 默认值
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, 0, cfg.Redis.DB)

	// 验证 Database 默认值
	assert.Equal(t, "postgres", cfg.Database.Driver)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)

	// 验证 Log 默认值
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}

// =============================================================================
// 🔧 Loader 测试
// =============================================================================

func TestLoader_LoadDefaults(t *testing.T) {
	// 不指定配置文件，应该返回默认值
	cfg, err := NewLoader().Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 8080, cfg.Server.HTTPPort)
	assert.Equal(t, "default-agent", cfg.Agent.Name)
}

func TestLoader_LoadFromYAML(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
server:
  http_port: 8888
  grpc_port: 9999
  read_timeout: 60s

agent:
  name: "test-agent"
  model: "claude-3"
  max_iterations: 20
  temperature: 0.5
  memory:
    enabled: true
    type: "vector"
    max_messages: 200

redis:
  addr: "redis.example.com:6379"
  password: "secret"
  db: 1

log:
  level: "debug"
  format: "console"
`
	err := os.WriteFile(configPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// 加载配置
	cfg, err := NewLoader().
		WithConfigPath(configPath).
		Load()
	require.NoError(t, err)

	// 验证 YAML 值覆盖了默认值
	assert.Equal(t, 8888, cfg.Server.HTTPPort)
	assert.Equal(t, 9999, cfg.Server.GRPCPort)
	assert.Equal(t, 60*time.Second, cfg.Server.ReadTimeout)

	assert.Equal(t, "test-agent", cfg.Agent.Name)
	assert.Equal(t, "claude-3", cfg.Agent.Model)
	assert.Equal(t, 20, cfg.Agent.MaxIterations)
	assert.Equal(t, 0.5, cfg.Agent.Temperature)
	assert.Equal(t, "vector", cfg.Agent.Memory.Type)
	assert.Equal(t, 200, cfg.Agent.Memory.MaxMessages)

	assert.Equal(t, "redis.example.com:6379", cfg.Redis.Addr)
	assert.Equal(t, "secret", cfg.Redis.Password)
	assert.Equal(t, 1, cfg.Redis.DB)

	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "console", cfg.Log.Format)
}

func TestLoader_LoadFromEnv(t *testing.T) {
	// 设置环境变量
	envVars := map[string]string{
		"AGENTFLOW_SERVER_HTTP_PORT":    "7777",
		"AGENTFLOW_SERVER_GRPC_PORT":    "8888",
		"AGENTFLOW_AGENT_NAME":          "env-agent",
		"AGENTFLOW_AGENT_MODEL":         "gpt-4-turbo",
		"AGENTFLOW_AGENT_MAX_ITERATIONS": "15",
		"AGENTFLOW_AGENT_TEMPERATURE":   "0.9",
		"AGENTFLOW_REDIS_ADDR":          "env-redis:6379",
		"AGENTFLOW_LOG_LEVEL":           "warn",
	}

	// 设置环境变量
	for k, v := range envVars {
		os.Setenv(k, v)
	}
	// 清理环境变量
	defer func() {
		for k := range envVars {
			os.Unsetenv(k)
		}
	}()

	// 加载配置
	cfg, err := NewLoader().Load()
	require.NoError(t, err)

	// 验证环境变量覆盖了默认值
	assert.Equal(t, 7777, cfg.Server.HTTPPort)
	assert.Equal(t, 8888, cfg.Server.GRPCPort)
	assert.Equal(t, "env-agent", cfg.Agent.Name)
	assert.Equal(t, "gpt-4-turbo", cfg.Agent.Model)
	assert.Equal(t, 15, cfg.Agent.MaxIterations)
	assert.Equal(t, 0.9, cfg.Agent.Temperature)
	assert.Equal(t, "env-redis:6379", cfg.Redis.Addr)
	assert.Equal(t, "warn", cfg.Log.Level)
}

func TestLoader_EnvOverridesYAML(t *testing.T) {
	// 创建临时配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
server:
  http_port: 8888
agent:
  name: "yaml-agent"
  model: "yaml-model"
`
	err := os.WriteFile(configPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// 设置环境变量（应该覆盖 YAML）
	os.Setenv("AGENTFLOW_SERVER_HTTP_PORT", "9999")
	os.Setenv("AGENTFLOW_AGENT_NAME", "env-agent")
	defer func() {
		os.Unsetenv("AGENTFLOW_SERVER_HTTP_PORT")
		os.Unsetenv("AGENTFLOW_AGENT_NAME")
	}()

	// 加载配置
	cfg, err := NewLoader().
		WithConfigPath(configPath).
		Load()
	require.NoError(t, err)

	// 环境变量应该覆盖 YAML
	assert.Equal(t, 9999, cfg.Server.HTTPPort)
	assert.Equal(t, "env-agent", cfg.Agent.Name)
	// YAML 值应该保留（没有被环境变量覆盖）
	assert.Equal(t, "yaml-model", cfg.Agent.Model)
}

func TestLoader_CustomEnvPrefix(t *testing.T) {
	// 设置自定义前缀的环境变量
	os.Setenv("MYAPP_SERVER_HTTP_PORT", "6666")
	os.Setenv("MYAPP_AGENT_NAME", "custom-prefix-agent")
	defer func() {
		os.Unsetenv("MYAPP_SERVER_HTTP_PORT")
		os.Unsetenv("MYAPP_AGENT_NAME")
	}()

	// 使用自定义前缀加载
	cfg, err := NewLoader().
		WithEnvPrefix("MYAPP").
		Load()
	require.NoError(t, err)

	assert.Equal(t, 6666, cfg.Server.HTTPPort)
	assert.Equal(t, "custom-prefix-agent", cfg.Agent.Name)
}

func TestLoader_WithValidator(t *testing.T) {
	// 添加验证器
	validator := func(cfg *Config) error {
		if cfg.Server.HTTPPort < 1024 {
			return assert.AnError
		}
		return nil
	}

	// 设置无效端口
	os.Setenv("AGENTFLOW_SERVER_HTTP_PORT", "80")
	defer os.Unsetenv("AGENTFLOW_SERVER_HTTP_PORT")

	// 加载应该失败
	_, err := NewLoader().
		WithValidator(validator).
		Load()
	assert.Error(t, err)
}

func TestLoader_NonExistentFile(t *testing.T) {
	// 指定不存在的文件，应该使用默认值（不报错）
	cfg, err := NewLoader().
		WithConfigPath("/non/existent/path/config.yaml").
		Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// 应该返回默认值
	assert.Equal(t, 8080, cfg.Server.HTTPPort)
}

func TestLoader_InvalidYAML(t *testing.T) {
	// 创建无效的 YAML 文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidYAML := `
server:
  http_port: [invalid
  this is not valid yaml
`
	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	require.NoError(t, err)

	// 加载应该失败
	_, err = NewLoader().
		WithConfigPath(configPath).
		Load()
	assert.Error(t, err)
}

// =============================================================================
// 🔍 Config 方法测试
// =============================================================================

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "invalid HTTP port (negative)",
			modify: func(c *Config) {
				c.Server.HTTPPort = -1
			},
			wantErr: true,
		},
		{
			name: "invalid HTTP port (too large)",
			modify: func(c *Config) {
				c.Server.HTTPPort = 70000
			},
			wantErr: true,
		},
		{
			name: "invalid max iterations",
			modify: func(c *Config) {
				c.Agent.MaxIterations = 0
			},
			wantErr: true,
		},
		{
			name: "invalid temperature (negative)",
			modify: func(c *Config) {
				c.Agent.Temperature = -0.5
			},
			wantErr: true,
		},
		{
			name: "invalid temperature (too high)",
			modify: func(c *Config) {
				c.Agent.Temperature = 3.0
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDatabaseConfig_DSN(t *testing.T) {
	tests := []struct {
		name     string
		config   DatabaseConfig
		expected string
	}{
		{
			name: "postgres DSN",
			config: DatabaseConfig{
				Driver:   "postgres",
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Password: "pass",
				Name:     "dbname",
				SSLMode:  "disable",
			},
			expected: "host=localhost port=5432 user=user password=pass dbname=dbname sslmode=disable",
		},
		{
			name: "mysql DSN",
			config: DatabaseConfig{
				Driver:   "mysql",
				Host:     "localhost",
				Port:     3306,
				User:     "user",
				Password: "pass",
				Name:     "dbname",
			},
			expected: "user:pass@tcp(localhost:3306)/dbname?parseTime=true",
		},
		{
			name: "sqlite DSN",
			config: DatabaseConfig{
				Driver: "sqlite",
				Name:   "/path/to/db.sqlite",
			},
			expected: "/path/to/db.sqlite",
		},
		{
			name: "unknown driver",
			config: DatabaseConfig{
				Driver: "unknown",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.DSN())
		})
	}
}

// =============================================================================
// 🚀 MustLoad 测试
// =============================================================================

func TestMustLoad_Success(t *testing.T) {
	// 创建有效配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
server:
  http_port: 8080
`
	err := os.WriteFile(configPath, []byte(yamlContent), 0644)
	require.NoError(t, err)

	// 不应该 panic
	assert.NotPanics(t, func() {
		cfg := MustLoad(configPath)
		assert.Equal(t, 8080, cfg.Server.HTTPPort)
	})
}

func TestMustLoad_InvalidFile(t *testing.T) {
	// 创建无效配置文件
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	err := os.WriteFile(configPath, []byte("invalid: [yaml"), 0644)
	require.NoError(t, err)

	// 应该 panic
	assert.Panics(t, func() {
		MustLoad(configPath)
	})
}

func TestLoadFromEnv_Function(t *testing.T) {
	os.Setenv("AGENTFLOW_AGENT_NAME", "env-only-agent")
	defer os.Unsetenv("AGENTFLOW_AGENT_NAME")

	cfg, err := LoadFromEnv()
	require.NoError(t, err)
	assert.Equal(t, "env-only-agent", cfg.Agent.Name)
}

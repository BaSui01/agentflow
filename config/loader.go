// =============================================================================
// 📦 AgentFlow 配置加载器
// =============================================================================
// 统一配置加载，支持 YAML 文件 + 环境变量覆盖
//
// 使用方法:
//
//	cfg, err := config.NewLoader().
//	    WithConfigPath("config.yaml").
//	    WithEnvPrefix("AGENTFLOW").
//	    Load()
//
// 配置优先级: 默认值 → YAML 文件 → 环境变量
// =============================================================================
package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// =============================================================================
// 🎯 核心配置结构
// =============================================================================

// Config 是 AgentFlow 的完整配置结构
type Config struct {
	// Server 服务器配置
	Server ServerConfig `yaml:"server" env:"SERVER"`

	// Agent 默认 Agent 配置
	Agent AgentConfig `yaml:"agent" env:"AGENT"`

	// Redis 缓存配置
	Redis RedisConfig `yaml:"redis" env:"REDIS"`

	// Database 数据库配置
	Database DatabaseConfig `yaml:"database" env:"DATABASE"`

	// Qdrant 向量存储配置
	Qdrant QdrantConfig `yaml:"qdrant" env:"QDRANT"`

	// LLM 大语言模型配置
	LLM LLMConfig `yaml:"llm" env:"LLM"`

	// Log 日志配置
	Log LogConfig `yaml:"log" env:"LOG"`

	// Telemetry 遥测配置
	Telemetry TelemetryConfig `yaml:"telemetry" env:"TELEMETRY"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	// HTTP 端口
	HTTPPort int `yaml:"http_port" env:"HTTP_PORT"`
	// gRPC 端口
	GRPCPort int `yaml:"grpc_port" env:"GRPC_PORT"`
	// Metrics 端口
	MetricsPort int `yaml:"metrics_port" env:"METRICS_PORT"`
	// 读取超时
	ReadTimeout time.Duration `yaml:"read_timeout" env:"READ_TIMEOUT"`
	// 写入超时
	WriteTimeout time.Duration `yaml:"write_timeout" env:"WRITE_TIMEOUT"`
	// 优雅关闭超时
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" env:"SHUTDOWN_TIMEOUT"`
}

// AgentConfig Agent 配置（与 types.AgentConfig 兼容）
type AgentConfig struct {
	// 名称
	Name string `yaml:"name" env:"NAME"`
	// 描述
	Description string `yaml:"description" env:"DESCRIPTION"`
	// 模型名称
	Model string `yaml:"model" env:"MODEL"`
	// 系统提示词
	SystemPrompt string `yaml:"system_prompt" env:"SYSTEM_PROMPT"`
	// 最大迭代次数
	MaxIterations int `yaml:"max_iterations" env:"MAX_ITERATIONS"`
	// 温度参数
	Temperature float64 `yaml:"temperature" env:"TEMPERATURE"`
	// 最大 Token 数
	MaxTokens int `yaml:"max_tokens" env:"MAX_TOKENS"`
	// 超时时间
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
	// 是否启用流式输出
	StreamEnabled bool `yaml:"stream_enabled" env:"STREAM_ENABLED"`
	// 记忆配置
	Memory MemoryConfig `yaml:"memory" env:"MEMORY"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	// 是否启用
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// 类型: buffer, summary, vector
	Type string `yaml:"type" env:"TYPE"`
	// 最大消息数
	MaxMessages int `yaml:"max_messages" env:"MAX_MESSAGES"`
	// Token 限制
	TokenLimit int `yaml:"token_limit" env:"TOKEN_LIMIT"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	// 地址
	Addr string `yaml:"addr" env:"ADDR"`
	// 密码
	Password string `yaml:"password" env:"PASSWORD"`
	// 数据库编号
	DB int `yaml:"db" env:"DB"`
	// 连接池大小
	PoolSize int `yaml:"pool_size" env:"POOL_SIZE"`
	// 最小空闲连接
	MinIdleConns int `yaml:"min_idle_conns" env:"MIN_IDLE_CONNS"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	// 驱动类型: postgres, mysql, sqlite
	Driver string `yaml:"driver" env:"DRIVER"`
	// 主机
	Host string `yaml:"host" env:"HOST"`
	// 端口
	Port int `yaml:"port" env:"PORT"`
	// 用户名
	User string `yaml:"user" env:"USER"`
	// 密码
	Password string `yaml:"password" env:"PASSWORD"`
	// 数据库名
	Name string `yaml:"name" env:"NAME"`
	// SSL 模式
	SSLMode string `yaml:"ssl_mode" env:"SSL_MODE"`
	// 最大连接数
	MaxOpenConns int `yaml:"max_open_conns" env:"MAX_OPEN_CONNS"`
	// 最大空闲连接
	MaxIdleConns int `yaml:"max_idle_conns" env:"MAX_IDLE_CONNS"`
	// 连接最大生命周期
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" env:"CONN_MAX_LIFETIME"`
}

// QdrantConfig Qdrant 向量存储配置
type QdrantConfig struct {
	// 主机
	Host string `yaml:"host" env:"HOST"`
	// gRPC 端口
	Port int `yaml:"port" env:"PORT"`
	// API Key（可选）
	APIKey string `yaml:"api_key" env:"API_KEY"`
	// 默认集合名
	Collection string `yaml:"collection" env:"COLLECTION"`
}

// LLMConfig LLM 配置
type LLMConfig struct {
	// 默认 Provider
	DefaultProvider string `yaml:"default_provider" env:"DEFAULT_PROVIDER"`
	// API Key（通用）
	APIKey string `yaml:"api_key" env:"API_KEY"`
	// 基础 URL（可选）
	BaseURL string `yaml:"base_url" env:"BASE_URL"`
	// 请求超时
	Timeout time.Duration `yaml:"timeout" env:"TIMEOUT"`
	// 最大重试次数
	MaxRetries int `yaml:"max_retries" env:"MAX_RETRIES"`
}

// LogConfig 日志配置
type LogConfig struct {
	// 日志级别: debug, info, warn, error
	Level string `yaml:"level" env:"LEVEL"`
	// 输出格式: json, console
	Format string `yaml:"format" env:"FORMAT"`
	// 输出路径
	OutputPaths []string `yaml:"output_paths" env:"OUTPUT_PATHS"`
	// 是否启用调用者信息
	EnableCaller bool `yaml:"enable_caller" env:"ENABLE_CALLER"`
	// 是否启用堆栈跟踪
	EnableStacktrace bool `yaml:"enable_stacktrace" env:"ENABLE_STACKTRACE"`
}

// TelemetryConfig 遥测配置
type TelemetryConfig struct {
	// 是否启用
	Enabled bool `yaml:"enabled" env:"ENABLED"`
	// OTLP 端点
	OTLPEndpoint string `yaml:"otlp_endpoint" env:"OTLP_ENDPOINT"`
	// 服务名称
	ServiceName string `yaml:"service_name" env:"SERVICE_NAME"`
	// 采样率
	SampleRate float64 `yaml:"sample_rate" env:"SAMPLE_RATE"`
}

// =============================================================================
// 🔧 配置加载器
// =============================================================================

// Loader 配置加载器（Builder 模式）
type Loader struct {
	configPath string
	envPrefix  string
	validators []func(*Config) error
}

// NewLoader 创建新的配置加载器
func NewLoader() *Loader {
	return &Loader{
		envPrefix:  "AGENTFLOW",
		validators: make([]func(*Config) error, 0),
	}
}

// WithConfigPath 设置配置文件路径
func (l *Loader) WithConfigPath(path string) *Loader {
	l.configPath = path
	return l
}

// WithEnvPrefix 设置环境变量前缀
func (l *Loader) WithEnvPrefix(prefix string) *Loader {
	l.envPrefix = prefix
	return l
}

// WithValidator 添加配置验证器
func (l *Loader) WithValidator(v func(*Config) error) *Loader {
	l.validators = append(l.validators, v)
	return l
}

// Load 加载配置
// 优先级: 默认值 → YAML 文件 → 环境变量
func (l *Loader) Load() (*Config, error) {
	// 1. 从默认值开始
	cfg := DefaultConfig()

	// 2. 如果指定了配置文件，从文件加载
	if l.configPath != "" {
		if err := l.loadFromFile(cfg); err != nil {
			return nil, fmt.Errorf("failed to load config from file: %w", err)
		}
	}

	// 3. 从环境变量覆盖
	if err := l.loadFromEnv(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from env: %w", err)
	}

	// 4. 运行验证器
	for _, v := range l.validators {
		if err := v(cfg); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}
	}

	return cfg, nil
}

// loadFromFile 从 YAML 文件加载配置
func (l *Loader) loadFromFile(cfg *Config) error {
	data, err := os.ReadFile(l.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，使用默认值
			return nil
		}
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// loadFromEnv 从环境变量加载配置
func (l *Loader) loadFromEnv(cfg *Config) error {
	return l.setFieldsFromEnv(reflect.ValueOf(cfg).Elem(), l.envPrefix)
}

// setFieldsFromEnv 递归设置结构体字段
func (l *Loader) setFieldsFromEnv(v reflect.Value, prefix string) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		// 获取 env tag
		envTag := fieldType.Tag.Get("env")
		if envTag == "" || envTag == "-" {
			continue
		}

		envKey := prefix + "_" + envTag

		// 如果是结构体，递归处理
		if field.Kind() == reflect.Struct {
			if err := l.setFieldsFromEnv(field, envKey); err != nil {
				return err
			}
			continue
		}

		// 获取环境变量值
		envValue := os.Getenv(envKey)
		if envValue == "" {
			continue
		}

		// 设置字段值
		if err := setFieldValue(field, envValue); err != nil {
			return fmt.Errorf("failed to set %s: %w", envKey, err)
		}
	}

	return nil
}

// setFieldValue 设置字段值
func setFieldValue(field reflect.Value, value string) error {
	if !field.CanSet() {
		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(value)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// 特殊处理 time.Duration
		if field.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			field.SetInt(int64(d))
		} else {
			i, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return err
			}
			field.SetInt(i)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		u, err := strconv.ParseUint(value, 10, 64)
		if err != nil {
			return err
		}
		field.SetUint(u)

	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return err
		}
		field.SetFloat(f)

	case reflect.Bool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		field.SetBool(b)

	case reflect.Slice:
		// 支持逗号分隔的字符串切片
		if field.Type().Elem().Kind() == reflect.String {
			parts := strings.Split(value, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			field.Set(reflect.ValueOf(parts))
		}
	}

	return nil
}

// =============================================================================
// 🔍 辅助函数
// =============================================================================

// MustLoad 加载配置，失败时 panic
func MustLoad(path string) *Config {
	cfg, err := NewLoader().WithConfigPath(path).Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	return cfg
}

// LoadFromEnv 仅从环境变量加载配置
func LoadFromEnv() (*Config, error) {
	return NewLoader().Load()
}

// Validate 验证配置
func (c *Config) Validate() error {
	var errs []string

	// 验证服务器配置
	if c.Server.HTTPPort <= 0 || c.Server.HTTPPort > 65535 {
		errs = append(errs, "invalid HTTP port")
	}

	// 验证 Agent 配置
	if c.Agent.MaxIterations <= 0 {
		errs = append(errs, "max_iterations must be positive")
	}
	if c.Agent.Temperature < 0 || c.Agent.Temperature > 2 {
		errs = append(errs, "temperature must be between 0 and 2")
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// DSN 返回数据库连接字符串
func (d *DatabaseConfig) DSN() string {
	switch d.Driver {
	case "postgres":
		return fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
			d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
		)
	case "mysql":
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true",
			d.User, d.Password, d.Host, d.Port, d.Name,
		)
	case "sqlite":
		return d.Name
	default:
		return ""
	}
}

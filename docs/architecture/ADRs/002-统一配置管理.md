# ADR 002: 统一配置管理

## 状态
- **状态**: 已接受
- **日期**: 2026-01-29
- **作者**: AgentFlow Team

## 背景

AgentFlow 框架包含多个模块（LLM、Agent、RAG、Workflow 等），每个模块都有自己的配置需求。之前配置分散在各处，导致：

1. 配置重复定义
2. 环境变量管理混乱
3. 配置验证困难
4. 缺乏统一的配置加载机制

## 决策

我们采用**统一配置管理方案**，设计一个集中式的配置结构。

### 配置结构

```go
// AppConfig 是统一的配置根结构
type AppConfig struct {
    Version       string                // 配置版本
    Metadata      ConfigMetadata        // 配置元数据
    Server        ServerConfig          // 服务器配置
    LLM           LLMConfig             // LLM 模块配置
    Agent         AgentConfig           // Agent 模块配置
    RAG           RAGConfig             // RAG 模块配置
    Workflow      WorkflowConfig        // 工作流配置
    Storage       StorageConfig         // 存储配置
    Observability ObservabilityConfig   // 可观测性配置
    Security      SecurityConfig        // 安全配置
}
```

### 配置加载优先级

```
默认值 → 配置文件 (YAML/JSON) → 环境变量
```

### 环境变量命名规范

- 前缀: `AGENTFLOW_`
- 分层: `AGENTFLOW_<MODULE>_<KEY>`
- 示例:
  - `AGENTFLOW_SERVER_PORT=8080`
  - `AGENTFLOW_LLM_DEFAULT_PROVIDER=openai`
  - `AGENTFLOW_DB_HOST=localhost`

### 配置文件搜索路径

1. `AGENTFLOW_CONFIG` 环境变量指定的路径
2. `./agentflow.yaml`
3. `./config/agentflow.yaml`
4. `/etc/agentflow/config.yaml`

## 后果

### 优点

- ✅ **单一 truth source**: 所有配置在一个地方管理
- ✅ **类型安全**: Go 结构体提供编译时检查
- ✅ **文档化**: 配置结构即文档
- ✅ **热重载**: 支持配置文件变更自动加载
- ✅ **验证**: 统一的配置验证机制

### 缺点

- ❌ **配置结构可能变大**: 需要良好的组织
- ❌ **向后兼容**: 修改配置结构需要考虑兼容性

## 实现细节

### 配置加载器

```go
loader := config.NewLoader()
cfg, err := loader.
    WithConfigPath("config.yaml").
    WithEnvPrefix("AGENTFLOW").
    Load()
```

### 配置验证

```go
func (c *AppConfig) Validate() error {
    if c.Server.Port <= 0 {
        return fmt.Errorf("invalid server port")
    }
    // ...
}
```

### 环境变量解析

支持以下类型：
- `string`: 直接赋值
- `int`: 解析为整数
- `bool`: 解析为布尔值
- `time.Duration`: 解析为时长 (e.g., "30s", "5m")
- `[]string`: 逗号分隔的字符串

## 相关决策

- ADR 001: 分层架构设计
- ADR 003: 模块接口设计

## 参考

- [Viper](https://github.com/spf13/viper) - Go 配置管理库
- [Koanf](https://github.com/knadh/koanf) - 轻量级配置管理

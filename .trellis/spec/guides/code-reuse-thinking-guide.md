# Code Reuse Thinking Guide

> **Purpose**: 在编写新代码前停下来思考——是否已经存在类似的代码？

---

## The Problem

**重复代码是 Bug 的第一大来源。**

当你复制粘贴或重写现有逻辑时：
- Bug 修复不会同步传播
- 行为随时间分化
- 代码库变得越来越难以理解

---

## Before Writing New Code

### Step 1: 优先搜索现有代码

```bash
# 搜索相似的函数名
grep -r "func.*Builder" --include="*.go" | grep -v "_test.go"

# 搜索相似逻辑模式
grep -r "WrapError" --include="*.go" | head -10

# 搜索 AgentFlow 特定模式
grep -rn "Adapter" ./agent/adapters --include="*.go"
grep -rn "Factory" ./llm --include="*.go" | grep "func NEW"
```

### Step 2: AgentFlow 项目搜索清单

在 AgentFlow 项目中，搜索这些关键模式：

| 你想写的代码 | 搜索位置 | 示例 |
|--------------|----------|------|
| Builder 模式 | `grep -rn "func.*Builder" ./agent` | `NewAgentBuilder` |
| Adapter 模式 | `grep -rn "Adapter" ./agent/adapters` | `ChatRequestAdapter` |
| Factory 模式 | `grep -rn "Factory" ./llm ./agent/core` | `ProviderFactory` |
| 错误包装 | `grep -rn "WrapError" ./types` | `WrapError`, `NewError` |
| 日志字段 | `grep -rn "zap\.(String\|Int\|Error)"` | `zap.String` |
| 结构体标签 | `grep -rn "bson:\"_id\"" ./agent/persistence` | MongoDB 文档模式 |

### Step 3: 问自己这些问题

| 问题 | 如果答案是 Yes... |
|------|-------------------|
| 是否有类似函数存在？ | 使用或扩展现有函数 |
| 这个模式在其他地方使用吗？ | 遵循现有模式 |
| 这可以是共享工具吗？ | 在正确的位置创建它 |
| 我正在从另一个文件复制代码吗？ | **STOP** - 提取为共享代码 |

---

## AgentFlow 项目复用模式

### 模式 1: Builder 模式 (Agent)

**位置**: `agent/builder.go`

**正确做法**:
```go
// ✅ 复用现有的 Builder 模式
import "github.com/BaSui01/agentflow/agent"

agent, err := agent.NewBuilder().
    WithModel("gpt-4").
    WithTools(tools).
    Build()
```

**错误做法**:
```go
// ❌ 不要这样做 - 直接构造，遗漏默认值
agent := &Agent{
    ID:    generateID(),
    Model: "gpt-4",
    // ... 遗漏了 Tools、Config 等字段
}
```

**搜索命令**:
```bash
grep -rn "func.*With[A-Z]" ./agent/builder.go
grep -rn "type.*Builder struct" ./agent --include="*.go"
```

---

### 模式 2: Adapter 模式 (请求转换)

**位置**: `agent/adapters/chat_request_adapter.go`

**正确做法**:
```go
// ✅ 使用现有的 Adapter
import "github.com/BaSui01/agentflow/agent/adapters"

adapter := adapters.NewDefaultChatRequestAdapter()
req, err := adapter.Build(options, messages)
```

**错误做法**:
```go
// ❌ 不要这样做 - 直接构造 Provider 请求
req := &openai.ChatRequest{
    Model: "gpt-4",
    // ... 与项目约定不一致
}
```

**搜索命令**:
```bash
grep -rn "type.*Adapter interface" ./agent/adapters --include="*.go"
grep -rn "func NEW.*Adapter" ./agent/adapters --include="*.go"
```

---

### 模式 3: Factory 模式 (Provider)

**位置**: `llm/providers/factory.go`

**正确做法**:
```go
// ✅ 使用 Factory 创建 Provider
import "github.com/BaSui01/agentflow/llm"

factory := llm.NewDefaultProviderFactory()
provider, err := factory.Create("openai", config)
```

**错误做法**:
```go
// ❌ 不要这样做 - 直接实例化具体 Provider
provider := &openai.Provider{
    APIKey: config.APIKey,
    // ... 遗漏了配置验证和默认值
}
```

**搜索命令**:
```bash
grep -rn "type.*Factory interface" ./llm --include="*.go"
grep -rn "func NEW.*Factory" ./llm ./agent/core --include="*.go"
```

---

### 模式 4: 错误处理模式

**位置**: `types/error.go`

**正确做法**:
```go
// ✅ 使用 WrapError 保留错误链
import "github.com/BaSui01/agentflow/types"

result, err := client.Call(ctx, req)
if err != nil {
    return nil, types.WrapError(err, types.ErrUpstreamError, "provider call failed")
}
```

**错误做法**:
```go
// ❌ 不要这样做 - 丢失原始错误
result, err := client.Call(ctx, req)
if err != nil {
    return nil, errors.New("provider call failed")  // 原始错误丢失!
}
```

**搜索命令**:
```bash
grep -rn "WrapError\|NewError" ./types/error.go
grep -rn "types\.WrapError" --include="*.go" | head -10
```

---

### 模式 5: 结构化日志

**位置**: 项目各处使用 Zap

**正确做法**:
```go
// ✅ 使用结构化字段
import "go.uber.org/zap"

logger.Info("agent started",
    zap.String("agent_id", agent.ID),
    zap.String("model", agent.Model),
    zap.Int("tool_count", len(agent.Tools)),
)
```

**错误做法**:
```go
// ❌ 不要这样做 - 字符串拼接
logger.Info(fmt.Sprintf("agent %s started with model %s", agent.ID, agent.Model))
```

**搜索命令**:
```bash
grep -rn "zap\.\(String\|Int\|Error\|Duration\)" --include="*.go" | head -20
```

---

### 模式 6: MongoDB 文档结构

**位置**: `agent/persistence/mongodb/*.go`

**正确做法**:
```go
// ✅ 遵循现有的文档结构模式
type ConversationDocument struct {
    ID        string    `bson:"_id" json:"id"`
    AgentID   string    `bson:"agent_id" json:"agent_id"`
    CreatedAt time.Time `bson:"created_at" json:"created_at"`
    UpdatedAt time.Time `bson:"updated_at" json:"updated_at"`
}
```

**搜索命令**:
```bash
grep -rn "type.*Document struct" ./agent/persistence --include="*.go"
grep -rn "bson:\"_id\"" --include="*.go"
```

---

## Common Duplication Patterns in AgentFlow

### 模式 1: 复制验证函数

**错误做法**: 在不同的 Handler 中复制验证逻辑

**正确做法**: 提取到 `types/validation.go` 或共享包

```go
// types/validation.go
func ValidateModelName(name string) error {
    if name == "" {
        return NewInvalidRequestError("model name cannot be empty")
    }
    return nil
}

// 所有地方使用
if err := types.ValidateModelName(req.Model); err != nil {
    return err
}
```

---

### 模式 2: 重复常量定义

**错误做法**: 在多个文件中定义相同的常量

```go
// ❌ file_a.go
const defaultTimeout = 30 * time.Second

// ❌ file_b.go
const timeout = 30 * time.Second  // 重复!
```

**正确做法**: 统一定义在 `types/constants.go`

```go
// types/constants.go
const (
    DefaultTimeout = 30 * time.Second
    MaxRetries    = 3
)
```

---

### 模式 3: 重复的错误转换

**错误做法**: 每个 Handler 自己转换错误

```go
// ❌ handler_a.go
if err != nil {
    c.JSON(500, gin.H{"error": err.Error()})
}

// ❌ handler_b.go
if err != nil {
    c.JSON(500, gin.H{"error": err.Error()})  // 重复!
}
```

**正确做法**: 使用 `api/error_mapping.go`

```go
// api/error_mapping.go
func ErrorInfoFromTypesError(err *types.Error, status int) *ErrorInfo {
    // 统一的错误转换逻辑
}

// 所有 Handler 使用
if err != nil {
    c.JSON(status, api.ErrorInfoFromTypesError(typedErr, status))
}
```

---

## When to Abstract

**应该抽象时**:
- 相同代码出现 3+ 次
- 逻辑复杂到可能有 Bug
- 多个人可能需要这个功能

**不应该抽象时**:
- 只使用一次
- 简单的一行代码
- 抽象会比重复更复杂

---

## After Batch Modifications

当你对多个文件做了类似的修改后：

1. **Review**: 是否遗漏了某些实例？
2. **Search**: 运行 grep 查找遗漏的
3. **Consider**: 是否应该抽象？

**AgentFlow 示例**:
```bash
# 修改后搜索是否遗漏
# 例如修改了 ModelOptions 字段后，检查所有 clone 函数
grep -rn "cloneAdapter\|ModelOptions" ./agent/adapters --include="*.go"
```

---

## Checklist Before Commit

- [ ] 搜索了现有类似代码
- [ ] 没有应该共享的复制粘贴逻辑
- [ ] 常量定义在一个地方
- [ ] 类似模式遵循相同结构
- [ ] 使用了项目的 Builder/Adapter/Factory 模式
- [ ] 错误处理遵循 `types.Error` 模式
- [ ] 日志使用结构化字段

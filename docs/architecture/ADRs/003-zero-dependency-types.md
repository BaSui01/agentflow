# ADR 003: 零依赖 Types 包

## 状态
- **状态**: 已接受
- **日期**: 2025-01-10
- **作者**: AgentFlow Team

## 背景

在 Go 项目中，循环依赖是一个常见问题。当多个包相互依赖时，会导致编译失败和代码组织混乱。AgentFlow 作为一个大型框架，需要特别注意这个问题。

## 决策

我们创建 `types` 包作为**零依赖的核心类型包**，并遵循以下规则：

### 规则 1: Types 包零依赖

```go
// types/message.go
package types

import (
    "encoding/json"  // ✅ 标准库允许
    "time"
    // ❌ 禁止导入任何 agentflow 内部包
)
```

### 规则 2: 所有包都可以依赖 Types

```
types ← llm
      ← agent
      ← rag
      ← workflow
      ← config
```

### 规则 3: Types 包含核心领域类型

- `Message`: 消息类型
- `ToolCall`: 工具调用
- `ToolSchema`: 工具定义
- `Error`: 错误类型
- `TokenUsage`: Token 使用统计

### 规则 4: 避免在 Types 中定义接口

Types 包应该主要包含数据结构，接口定义在各自的包中。

## 后果

### 优点

- ✅ **消除循环依赖**: 所有包共享基础类型
- ✅ **编译速度**: 减少依赖链长度
- ✅ **可重用性**: types 包可以被外部项目使用
- ✅ **清晰的边界**: 明确核心领域模型

### 缺点

- ❌ **类型扩展受限**: 不能在类型中添加方法依赖其他包
- ❌ **可能需要类型转换**: 各包可能需要包装 types

## 示例

### Types 包定义

```go
// types/message.go
package types

type Message struct {
    Role    Role   `json:"role"`
    Content string `json:"content"`
}

type Role string

const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
)
```

### LLM 包使用

```go
// llm/provider.go
package llm

import "github.com/BaSui01/agentflow/types"

type Provider interface {
    Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
}

type ChatRequest struct {
    Model    string         `json:"model"`
    Messages []types.Message `json:"messages"`  // ✅ 使用 types 包
}
```

## 相关决策

- ADR 001: 分层架构设计

## 参考

- [Go 代码组织](https://go.dev/doc/code)
- [Package Oriented Design](https://www.ardanlabs.com/blog/2017/02/package-oriented-design.html)

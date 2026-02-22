# A2A 协议 (Agent-to-Agent Protocol)

展示 Agent 间通信协议：Agent Card 生成、消息创建与验证、Client/Server 架构。

## 功能

- **Agent Card**：描述 Agent 能力、工具、输入输出 Schema 的元数据卡片
- **消息系统**：创建任务消息、回复消息，支持 JSON 序列化和验证
- **Client/Server**：HTTP 客户端和服务端的配置与端点说明
- **异步任务**：支持同步和异步消息处理模式

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/16_a2a_protocol
go run main.go
```

## 代码说明

`a2a.NewAgentCard` 创建 Agent 能力描述；`a2a.NewTaskMessage` 创建任务消息；`a2a.NewHTTPClient` / `a2a.NewHTTPServer` 提供 HTTP 通信层。服务端实现 `http.Handler` 接口，暴露 `.well-known/agent.json` 发现端点。

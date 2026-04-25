# Gemini 官方 SDK 迁移清理计划

## 目标

将项目内 Gemini / Google 相关运行时请求从手写 HTTP 适配切换到官方 `google.golang.org/genai` SDK，同时保持现有 AgentFlow 对外接口、工厂入口与 vendor profile 不变。

## 已锁定行为

- 迁移前已执行回归：
  - `go test ./llm/providers/gemini ./llm/capabilities/embedding ./llm/capabilities/image ./llm/capabilities/video ./llm/providers/vendor`

## 范围

- `llm/providers/gemini`
- `llm/capabilities/embedding/gemini.go`
- `llm/capabilities/image/gemini.go`
- `llm/capabilities/video/gemini.go`
- `llm/providers/vendor/gemini.go`
- 对应测试与架构文档

## 清理顺序

1. 提取共享的 Google GenAI client 构造，统一 API Key / Vertex / BaseURL / timeout 入口。
2. 替换 chat provider 的 Completion / Stream / ListModels / HealthCheck 到官方 SDK。
3. 替换 multimodal / embedding / image / video provider 的 Gemini 运行时调用到官方 SDK。
4. 删除被替代的 Gemini 手写请求路径拼装与直接 HTTP 调用。
5. 更新 ADR / 文档并执行回归验证。

## 风险点

- Vertex OAuth 与 Gemini API Key 两种认证路径都要保留。
- 需要保持 `llm.Provider`、`vendor.NewChatProviderFromConfig(...)`、multimodal builder 对外契约稳定。
- 图像流式、function call、structured output、safety settings、cached content 需要逐项映射。

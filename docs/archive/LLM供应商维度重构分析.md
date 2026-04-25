# LLM 供应商维度重构分析（含多智能体）

## 问题诊断
- 现状是“能力维度分包 + 供应商维度分包”并存：`llm/image`、`llm/speech`、`llm/embedding` 与 `llm/providers/*` 同时实现 OpenAI/Gemini/Anthropic，导致同一供应商配置重复散落。
- 接入层（如 `api/handlers/multimodal.go`）需要按能力手工组装 provider，扩展一个供应商要改多处。
- 多智能体流程（planner/executor）只能拿到单一 `llm.Provider`，无法自然继承同供应商的多模态能力与语言模型适配策略。

## 本次落地重构
- 新增 `llm/providers/vendor`，按供应商聚合能力：
  - `vendor.NewOpenAIProfile(...)`
  - `vendor.NewGeminiProfile(...)`
  - `vendor.NewAnthropicProfile(...)`
- 每个 `Profile` 统一承载：
  - `Chat`
  - `Embedding`
  - `Image`
  - `Video`
  - `TTS`
  - `STT`
  - `LanguageModels`（语言到模型映射）
- `api/handlers/multimodal.go` 改为通过供应商 `Profile` 装配，去掉 OpenAI/Gemini 在 handler 中按能力重复构建的逻辑。

## 多智能体影响分析
- 角色分工建议：
  - Planner：偏长上下文/推理模型（`profile.ModelForLanguage(lang, default)`）
  - Executor：偏工具调用与低延迟模型
  - Critic/Verifier：高一致性模型
- 统一供应商档案后，多智能体编排可直接按“供应商策略 + 角色策略”选择模型，不需要跨多个能力包查找配置。
- 结果：同一供应商下的对话、多模态、语音、语言适配可共享同一套凭证和默认模型策略，减少配置漂移。

## 下一步（建议）
1. 把 `llm/capability/factory` 迁移到 `vendor.Profile` 作为唯一构建入口，移除 capability 侧对 OpenAI/Gemini 的重复工厂分支。
2. 在 agent 编排层增加 `AgentRole -> (ProviderProfile, Model)` 映射，替代当前手写 planner/executor prompt 路径。
3. 为 `vendor.Profile` 增加能力探测与健康检查汇总，作为多智能体调度前置条件。

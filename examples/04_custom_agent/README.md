# 自定义 Agent (Custom Agent)

展示如何创建多个不同角色的自定义 Agent，每个 Agent 有独立的 PromptBundle 和行为特征。

## 功能

- 定义自定义 AgentType（代码审查、数据分析、故事创作）
- 为每个 Agent 配置专属的 PromptBundle（身份、策略、输出规则）
- 使用不同 Temperature 控制输出风格（审查 0.3 严谨，创作 0.9 发散）
- 通过 `ChatCompletion` 调用各 Agent

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`

## 运行

```bash
cd examples/04_custom_agent
go run main.go
```

## 代码说明

通过 `agent.Config` 的 `PromptBundle` 字段定义 Agent 的系统提示词，包括身份（Identity）、策略（Policies）和输出规则（OutputRules）。不同 Agent 使用不同的 Temperature 和 MaxTokens 配置。

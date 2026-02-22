# 安全护栏 (Guardrails)

展示 AgentFlow 的输入安全验证系统：PII 检测、注入攻击检测、验证器链。

## 功能

- **PII 检测**：识别并脱敏手机号、邮箱等个人信息
- **注入检测**：检测中英文 Prompt 注入攻击（角色劫持、指令覆盖）
- **验证器链**：按优先级串联多个验证器（长度、注入、关键词、PII），支持 CollectAll 模式

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/14_guardrails
go run main.go
```

## 代码说明

`guardrails.NewPIIDetector` 检测并脱敏 PII；`guardrails.NewInjectionDetector` 识别注入攻击；`guardrails.NewValidatorChain` 将多个验证器按优先级排序执行，支持 FailFast 和 CollectAll 两种模式。

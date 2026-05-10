## 🟡 优先级：MEDIUM（DRY 违规）

## 问题描述

12 个 LLM provider 中有 8 个各自维护 `multimodal.go`：

- `llm/providers/anthropic/multimodal.go`
- `llm/providers/doubao/multimodal.go`
- `llm/providers/gemini/multimodal.go`
- `llm/providers/glm/multimodal.go`
- `llm/providers/grok/multimodal.go`
- `llm/providers/minimax/multimodal.go`
- `llm/providers/mistral/multimodal.go`
- `llm/providers/openai/multimodal.go`
- `llm/providers/qwen/multimodal.go`

虽然已有 `llm/providers/base/multimodal_helpers.go` 提供共用工具函数，但 per-provider 多模态适配仍存在大量重复，新增 provider 需要复制 100+ 行代码。

## 建议方案

1. **抽取通用模板**：在 `llm/providers/base/` 中定义 `MultimodalAdapter` 模板，包含 image/audio/video 通用转换逻辑
2. **provider 级特化**：仅当 provider 有独特需求时（如 Gemini inline_data vs OpenAI image_url），override 特定方法
3. **统一测试**：为 base 模板写一套通用 multimodal 转换测试，所有 provider 通过 inline 注入测试自身适配是否符合规范

## TDD 流程建议

1. **Red**：在 `llm/providers/base/multimodal_template_test.go` 写一套 table-driven 测试，覆盖 image URL / image base64 / audio / video 场景
2. **Green**：让 base 实现满足所有用例
3. **Refactor**：依次替换每个 provider 的 multimodal.go，减少重复 ≥ 60%

## 验收指标
- 8 个 provider 的 `multimodal.go` 平均行数下降 ≥ 50%
- 新增 provider 时，multimodal 接入只需 ≤ 30 行代码

## 标签
`enhancement` `tech-debt`

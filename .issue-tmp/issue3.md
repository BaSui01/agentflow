## 🟡 优先级：MEDIUM（接口割裂）

## 问题描述

项目存在三套 Tokenizer 接口：

| 接口 | 位置 |
|------|------|
| `types.Tokenizer` | `types/token.go` |
| `llm/tokenizer.Tokenizer` | `llm/tokenizer/` |
| `rag.Tokenizer` | `rag/` |

`types/token.go` 第 30-37 行的注释明确承认"无法不引入循环依赖地统一"，并提供 `rag.NewLLMTokenizerAdapter` 适配器作为缓解方案。

但每次新增 provider 或新场景，开发者仍需要：
- 评估到底实现哪一个接口
- 可能需要手写适配器
- 维护负担分散在三个包

## 长期改进方向

### 方案 A（推荐）：抽象到独立子包
新增 `pkg/tokenizer/` 或 `internal/tokenizer/`，作为最底层依赖（无任何业务依赖），types/llm/rag 都依赖它，三个接口合并为一个。

### 方案 B：保留三接口，但加强契约
- 在 `types/token.go` 中明确注释三者职责差异
- 在 architecture_guard_test 中加规则，禁止三个 Tokenizer 之间互相 import
- 提供 `Validate(impl)` 通用契约测试帮助 provider 验证实现一致性

## TDD 流程建议

1. **Red**：在新 `pkg/tokenizer/contract_test.go` 中定义跨实现的契约测试（计数、编码、解码同步）
2. **Green**：所有现有实现注入到契约测试，跑通即认为方案可行
3. **Refactor**：迁移最常用的实现到新包

## 标签
`enhancement` `tech-debt`

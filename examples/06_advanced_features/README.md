# 高级功能 (Advanced Features)

展示 Agent 的高级能力：反射机制、动态工具选择、提示词工程。

## 功能

- **反射机制**：Agent 自我评估输出质量，迭代改进直到达标
- **动态工具选择**：根据任务语义自动评分和筛选最相关的工具
- **提示词工程**：提示词增强器、优化器、模板库的使用

## 前置条件

- Go 1.24+
- 环境变量 `OPENAI_API_KEY`（反射示例需要；工具选择和提示词工程无 API Key 也可运行）

## 运行

```bash
cd examples/06_advanced_features
go run main.go
```

## 代码说明

反射通过 `ReflectionExecutor` 实现多轮自我评估；工具选择通过 `DynamicToolSelector` 基于语义相似度、成本、可靠性评分；提示词工程包含 `PromptEnhancer`（增强）、`PromptOptimizer`（优化）和 `PromptTemplateLibrary`（模板库）。

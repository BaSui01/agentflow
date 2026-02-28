# 2026 前沿功能 (2026 Features)

展示分层记忆、GraphRAG、Agentic Browser、原生音频推理、Shadow AI 检测。

## 功能

- **分层记忆**：情景记忆（Episodic）+ 工作记忆（Working）+ 程序记忆（Procedural）
- **GraphRAG**：基于知识图谱的检索增强生成
- **Agentic Browser**：Agent 驱动的浏览器自动化（配置展示）
- **原生音频推理**：低延迟音频处理配置（配置展示）
- **Shadow AI 检测**：检测未授权的 AI API 调用和密钥泄露

## 前置条件

- Go 1.24+
- 无需 API Key

## 运行

```bash
cd examples/19_2026_features
go run main.go
```

## 代码说明

`memory.NewLayeredMemory` 创建三层记忆系统；`rag.NewKnowledgeGraph` 构建知识图谱；`guardrails.NewShadowAIDetector` 扫描域名和内容中的未授权 AI 使用。Browser 和 Audio 仅展示配置，需要外部驱动/提供商才能实际运行。

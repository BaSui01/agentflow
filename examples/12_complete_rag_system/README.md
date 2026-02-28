# 完整 RAG 系统 (Complete RAG System)

展示向量存储、混合检索、语义缓存、上下文压缩的完整 RAG 流程。

## 功能

- **向量存储**：InMemoryVectorStore 存储文档和嵌入向量
- **混合检索**：BM25 关键词匹配 + 向量语义搜索的加权融合
- **语义缓存**：基于向量相似度的查询缓存，避免重复检索
- **上下文压缩**：使用 ContextEngineer 管理对话上下文窗口

## 前置条件

- Go 1.24+
- 无需 API Key（使用内存向量存储和预计算嵌入）

## 运行

```bash
cd examples/12_complete_rag_system
go run main.go
```

## 代码说明

使用 `rag.NewHybridRetrieverWithVectorStore` 创建混合检索器，`rag.NewSemanticCache` 创建语义缓存，`agentcontext.New` 创建上下文管理器。文档使用预计算的 `[]float64` 嵌入向量。

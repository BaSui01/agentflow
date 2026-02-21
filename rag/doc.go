// Copyright 2025-2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

Package rag 提供检索增强生成（Retrieval-Augmented Generation）的完整实现。

该包覆盖 RAG 管线的全部阶段：文档分块、向量存储、混合检索、查询路由、
多跳推理、重排序、上下文检索和 Graph RAG，并提供工厂函数从全局配置
一键创建完整的检索管线。

# 核心接口/类型

  - VectorStore — 向量数据库统一接口（AddDocuments / Search / Delete / Update / Count）
  - LowLevelVectorStore — 底层向量存储接口，供 Graph RAG 和 memory 系统使用
  - Reranker — 重排序器接口（Simple / CrossEncoder / LLM 三种实现）
  - VectorIndex — 向量索引接口（Flat / HNSW 实现）
  - Tokenizer — RAG 分块专用分词器接口
  - ContextProvider — 上下文生成器接口，为 chunk 添加文档级上下文
  - EmbeddingProvider / RerankProvider — 外部 embedding 和 rerank 提供者接口
  - QueryLLMProvider — 基于 LLM 的查询处理接口

# 主要能力

  - 文档分块：固定大小、递归、语义、文档感知四种策略（DocumentChunker）
  - 混合检索：BM25 + 向量检索 + 可配置权重融合（HybridRetriever）
  - 查询变换：意图检测、查询重写、子查询分解、HyDE、Step-Back（QueryTransformer）
  - 查询路由：规则 + LLM 混合路由，自适应学习，支持 8 种检索策略（QueryRouter）
  - 多跳推理：多轮检索 + 推理链 + 去重 + 可视化（MultiHopReasoner）
  - 上下文检索：Anthropic 风格的 chunk 级上下文增强（ContextualRetrieval）
  - Graph RAG：知识图谱 + 向量检索混合（KnowledgeGraph / GraphRAG）
  - Web 增强检索：本地 RAG + 实时 Web 搜索融合（WebRetriever）
  - 向量存储后端：InMemory / Qdrant / Weaviate / Milvus / Pinecone
  - 语义缓存：基于向量相似度的查询结果缓存（SemanticCache）
  - 工厂函数：NewRetrieverFromConfig 等一键创建完整管线
*/
package rag

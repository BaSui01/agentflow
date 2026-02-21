// Copyright 2025-2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

Package loader 提供统一的 DocumentLoader 接口和内置文件加载器，
作为 RAG 管线的数据入口层。

它将原始数据源（本地文件、外部 API）与 rag.Document 类型桥接，
每个 Loader 读取特定格式并生成带有元数据的 []rag.Document，
供后续的 chunker、retriever 和 vector store 消费。

# 核心接口/类型

  - DocumentLoader — 统一加载接口（Load + SupportedTypes）
  - LoaderRegistry — 按文件扩展名路由到对应 Loader，支持自定义注册
  - TextLoader — 纯文本加载器（.txt）
  - MarkdownLoader — Markdown 加载器（.md），按一级标题拆分为多个 Document
  - CSVLoader — CSV 加载器（.csv），支持自定义分隔符和行分组
  - JSONLoader — JSON/JSONL 加载器（.json / .jsonl），支持字段映射
  - GitHubSourceAdapter — 将 sources.GitHubSource 适配为 DocumentLoader
  - ArxivSourceAdapter — 将 sources.ArxivSource 适配为 DocumentLoader

# 主要能力

  - 扩展名路由：LoaderRegistry 根据文件扩展名自动选择 Loader
  - 内置格式：开箱支持 .txt / .md / .csv / .json / .jsonl
  - 自定义扩展：通过 Registry.Register 注册任意扩展名的 Loader
  - 外部源适配：Adapter 模式将 GitHub / arXiv 等查询型数据源接入 Loader 体系

使用示例：

	registry := loader.NewLoaderRegistry()
	docs, err := registry.Load(ctx, "/path/to/data.csv")

	// 注册自定义 Loader
	registry.Register(".xml", myXMLLoader)
*/
package loader

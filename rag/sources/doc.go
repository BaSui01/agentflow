// Copyright 2025-2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by the project license.

/*
# 概述

Package sources 提供 RAG 外部知识源适配器，用于从第三方平台获取结构化数据
并转换为 RAG 管线可消费的格式。

每个 Source 封装了对应平台的 API 调用、重试逻辑和响应解析，
返回强类型的结果结构体，可通过 loader 包的 Adapter 转换为 rag.Document。

# 核心接口/类型

  - ArxivSource — arXiv 论文搜索适配器，解析 Atom XML 响应
  - ArxivPaper — arXiv 论文结构体（标题、摘要、作者、分类、PDF 链接等）
  - GitHubSource — GitHub 仓库和代码搜索适配器，支持 Token 认证
  - GitHubRepo — GitHub 仓库结构体（名称、描述、Stars、语言、Topics 等）
  - GitHubCodeResult — GitHub 代码搜索结果

# 主要能力

  - arXiv 论文搜索：按关键词和分类检索，支持排序、分页和自动重试
  - GitHub 仓库搜索：按关键词检索仓库，支持 Stars 和语言过滤
  - GitHub 代码搜索：按关键词和语言检索代码片段
  - GitHub README 获取：拉取指定仓库的 README 原始内容
  - 所有 HTTP 请求均使用 tlsutil.SecureHTTPClient 保障传输安全
*/
package sources

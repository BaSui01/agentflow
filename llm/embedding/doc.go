// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 embedding 提供统一的文本嵌入（Embedding）接口与多服务商实现，
用于将文本转换为向量表示以支持语义检索、分类与聚类等场景。

# 概述

不同嵌入服务商在 API 格式、认证方式与输入类型语义上存在差异。
本包通过 Provider 接口屏蔽这些差异，使上层业务可以在不修改调用
代码的前提下切换底层嵌入服务。

# 核心接口

  - Provider：统一嵌入接口，定义 Embed、EmbedQuery、EmbedDocuments 等方法。
  - EmbeddingRequest / EmbeddingResponse：标准化的请求与响应模型。
  - InputType：输入类型枚举，包括 query、document、classification、clustering 等。
  - BaseProvider：公共基类，封装 HTTP 请求、错误映射与批量辅助方法。

# 主要能力

  - 多服务商支持：内置 OpenAI、Voyage AI、Cohere、Jina AI、Google Gemini 五种实现。
  - 输入类型映射：自动将统一 InputType 转换为各服务商特定的任务类型参数。
  - 批量嵌入：各 Provider 支持批量输入，Gemini 额外支持 batchEmbedContents 端点。
  - 维度控制：支持 Matryoshka 维度（Jina）与可变维度（OpenAI）。
  - 安全 HTTP：通过 tlsutil.SecureHTTPClient 建立安全连接。

# 使用方式

	cfg := embedding.DefaultOpenAIConfig()
	cfg.APIKey = "sk-..."
	provider := embedding.NewOpenAIProvider(cfg)

	vec, err := provider.EmbedQuery(ctx, "搜索关键词")
	vecs, err := provider.EmbedDocuments(ctx, []string{"文档1", "文档2"})
*/
package embedding

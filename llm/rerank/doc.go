// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 rerank 提供统一的文档重排序接入层，支持多服务商适配与标准化
请求/响应模型。

# 概述

本包屏蔽不同重排序服务商在接口协议与评分语义上的差异，对上层
业务暴露一致的 Provider 接口。调用方只需构造 RerankRequest 即可
获得按相关性排序的文档列表，适用于 RAG 检索增强生成场景中的
候选文档精排。

典型使用场景：

  - 对向量检索返回的候选文档按 Query 相关性重排序。
  - 通过 TopN 参数截取最相关的前 N 篇文档。
  - 使用 RerankSimple 便捷方法快速完成纯文本列表的重排序。

# 核心接口

  - Provider：统一的重排序接口，包含 Rerank、RerankSimple、Name
    与 MaxDocuments 方法。
  - RerankRequest / RerankResponse：标准化的请求与响应模型。
  - Document：待排序文档，包含文本、ID 与标题。
  - RerankResult：单条排序结果，包含原始索引与归一化相关性分数。
  - RerankUsage：用量统计，包含 SearchUnits 与 Token 消耗。

# 主要能力

  - Cohere 适配：通过 CohereProvider 接入 Cohere Rerank v2 API。
  - Jina 适配：通过 JinaProvider 接入 Jina AI Reranker API，支持多语言模型。
  - Voyage 适配：通过 VoyageProvider 接入 Voyage AI Rerank API。
  - 配置管理：每个 Provider 提供独立的 Config 结构与 Default 工厂函数。
*/
package rerank

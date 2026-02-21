// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 image 提供统一的图像生成服务抽象，支持多种模型服务商的文生图、
图像编辑与变体生成能力。

# 概述

本包定义了图像生成领域的核心接口与数据模型，屏蔽不同服务商
（OpenAI DALL-E、Black Forest Labs Flux、Google Gemini 等）在
API 协议、参数格式和响应结构上的差异，对上层业务暴露一致的
请求/响应模型。

# 核心接口

  - Provider：图像生成提供者接口，包含 Generate（文生图）、
    Edit（图像编辑）、CreateVariation（变体生成）、Name 与
    SupportedSizes 五个方法。
  - GenerateRequest / GenerateResponse：生成请求与响应模型，
    支持 prompt、尺寸、质量、风格、种子等参数。
  - EditRequest / VariationRequest：编辑与变体请求模型。
  - ImageData / ImageUsage：图像数据与用量统计。

# 主要能力

  - 多 Provider 适配：内置 OpenAIProvider（DALL-E）、FluxProvider
    （Flux 2 系列）、GeminiProvider（Gemini 原生多模态）。
  - 文生图：通过文本 prompt 生成指定尺寸与风格的图像。
  - 图像编辑：基于原图与 mask 进行局部修改（OpenAI、Gemini）。
  - 变体生成：基于原图生成风格相近但细节不同的变体。
  - 异步轮询：Flux Provider 支持异步提交 + 轮询获取结果。
  - 配置体系：每个 Provider 提供独立的 Config 结构与 Default 函数。
*/
package image

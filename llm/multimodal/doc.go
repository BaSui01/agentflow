// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 multimodal 提供多模态内容的统一表示、格式转换与跨 Provider 路由能力，
支持文本、图像、音频、视频、文档等多种内容类型。

# 概述

本包解决两个核心问题：一是定义与模型服务商无关的多模态内容模型，
二是将统一模型自动转换为各 Provider（OpenAI、Anthropic、Gemini 等）
的专有格式。同时提供 Router 统一路由 embedding、rerank、TTS、STT、
image、video、music、3D、moderation 九大能力的 Provider 注册与调度。

# 核心接口

  - ContentType：内容类型枚举（text / image / audio / video / document）。
  - Content / MultimodalMessage：多模态内容项与消息模型，支持
    URL、Base64、元数据等多种载荷形式。
  - Processor：多模态格式转换器，将 MultimodalMessage 转换为
    OpenAI / Anthropic / Gemini / 通用格式的 llm.Message。
  - MultimodalProvider：Provider 包装器，为已有 llm.Provider
    透明注入多模态转换能力。
  - Router：多模态能力路由器，按 Capability 注册、查找与调度
    各类 Provider，支持默认 Provider 与按名称查找。

# 主要能力

  - 内容构建：NewTextContent / NewImageURLContent / NewImageBase64Content /
    NewAudioURLContent 等工厂函数快速构建内容项。
  - 文件加载：LoadImageFromFile / LoadImageFromURL / LoadAudioFromFile
    从本地文件或远程 URL 加载并自动编码。
  - 格式转换：Processor 按 Provider 名称自动选择目标格式。
  - 统一路由：Router 提供 RegisterXxx / Xxx / 便捷方法三层 API，
    覆盖九大 AI 能力的注册、获取与直接调用。
  - 能力探测：HasCapability / ListProviders 查询已注册能力。
*/
package multimodal

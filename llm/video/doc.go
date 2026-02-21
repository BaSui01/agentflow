// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 video 提供统一的视频生成与视频理解接口，适配 Gemini、Veo 和 Runway ML
三个服务商。

# 概述

本包将视频处理分为两大能力维度：视频分析（理解已有视频内容）和视频生成
（从文本/图像提示创建新视频）。不同 Provider 可能仅支持其中一种能力，
上层可通过 SupportsGeneration() 进行能力探测。

典型使用场景：

  - 使用 Gemini 多模态能力对视频进行内容理解与帧级分析。
  - 使用 Veo 3.1 或 Runway Gen-4 从文本/图像生成短视频。
  - 统一请求模型，在不同 Provider 间无缝切换。

# 核心接口

  - Provider — 视频处理的统一抽象，包含 Analyze()、Generate()、Name()、
    SupportedFormats() 与 SupportsGeneration() 方法。
  - AnalyzeRequest / AnalyzeResponse — 视频分析的请求与响应模型。
  - GenerateRequest / GenerateResponse — 视频生成的请求与响应模型。
  - FrameAnalysis / DetectedObject — 帧级分析结果与目标检测数据。
  - VideoData / VideoUsage — 生成视频元数据与用量统计。

# 主要能力

  - Gemini 适配：GeminiProvider 支持 MP4/WebM/MOV/AVI/MKV 五种格式的
    视频内容分析，基于 Gemini 多模态 API。
  - Veo 适配：VeoProvider 使用 Google Veo 3.1 生成视频，支持宽高比、
    负向提示、音频生成等参数。
  - Runway 适配：RunwayProvider 使用 Runway Gen-4 生成视频，支持
    image-to-video 与 text-to-video。
  - 异步轮询：Veo 与 Runway 均内置任务状态轮询机制。
  - 格式枚举：VideoFormat 常量覆盖 MP4、WebM、MOV、AVI、MKV。
*/
package video

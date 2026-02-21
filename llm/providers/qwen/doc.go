// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 qwen 提供阿里巴巴通义千问（Qwen）系列模型的 Provider 适配实现，
基于 OpenAI 兼容协议接入 DashScope API。

# 概述

Qwen Provider 复用 openaicompat 基础设施，通过 DashScope 的
compatible-mode 端点实现与 OpenAI API 格式一致的调用。默认模型为
qwen3-235b-a22b，支持文本补全、流式输出及多模态能力。

典型使用场景：

  - 文本补全与流式对话（Chat Completions）。
  - 使用 Wanx 系列模型生成图像（wanx-v1、wanx2.1-t2i-turbo 等）。
  - 使用 CosyVoice / Sambert 进行语音合成（TTS）。
  - 使用 text-embedding-v4 等模型创建文本嵌入。

# 核心接口

  - QwenProvider — 嵌入 openaicompat.Provider，继承补全与流式能力。
  - GenerateImage — 调用 Wanx 图像生成端点。
  - GenerateAudio — 调用 Qwen TTS 语音合成端点。
  - CreateEmbedding — 调用 Qwen Embedding 端点。

# 主要能力

  - OpenAI 兼容：基于 /compatible-mode/v1/ 端点，请求与响应格式与
    OpenAI API 保持一致，降低迁移成本。
  - 多模态支持：覆盖图像生成、语音合成与文本嵌入三类多模态任务。
  - 不支持的能力：视频生成、音频转录与微调（Fine-Tuning）当前返回
    NotSupportedError，便于上层做能力探测与降级。
*/
package qwen

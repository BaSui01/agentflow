// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 speech 提供统一的语音合成 (TTS) 与语音识别 (STT) 接入层，
支持多服务商适配与标准化请求/响应模型。

# 概述

本包将 TTS 与 STT 两大语音能力抽象为独立的 Provider 接口，
屏蔽不同服务商在音频格式、鉴权方式和响应结构上的差异。调用方
可通过统一的请求模型完成文本朗读、语音转写等任务。

典型使用场景：

  - 将文本转换为语音流或直接保存为音频文件。
  - 查询 Provider 可用的声音列表并按性别、语言筛选。
  - 将音频文件或音频流转写为文本，支持时间戳与说话人分离。
  - 获取词级别与片段级别的转录结果。

# 核心接口

  - TTSProvider：文本转语音接口，包含 Synthesize、SynthesizeToFile
    与 ListVoices 方法。
  - STTProvider：语音转文本接口，包含 Transcribe、TranscribeFile
    与 SupportedFormats 方法。
  - TTSRequest / TTSResponse：TTS 标准化请求与响应模型。
  - STTRequest / STTResponse：STT 标准化请求与响应模型。
  - Voice：可用声音描述，包含 ID、名称、语言与性别。
  - Segment / Word：转录片段与带时间戳的词级别结果。

# 主要能力

  - OpenAI TTS 适配：通过 OpenAITTSProvider 接入 OpenAI TTS API，
    支持 tts-1/tts-1-hd 模型与 6 种内置声音。
  - ElevenLabs TTS 适配：通过 ElevenLabsProvider 接入 ElevenLabs API，
    支持多语言模型与自定义声音。
  - OpenAI STT 适配：通过 OpenAISTTProvider 接入 Whisper API，
    支持多种音频格式与时间戳粒度。
  - Deepgram STT 适配：通过 DeepgramProvider 接入 Deepgram Nova API，
    支持 URL 转写、说话人分离与智能格式化。
  - 配置管理：每个 Provider 提供独立的 Config 结构与 Default 工厂函数。
*/
package speech

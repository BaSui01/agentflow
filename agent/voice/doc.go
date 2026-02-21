// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 voice 提供语音 Agent 与实时音频交互能力。

# 概述

voice 实现了完整的语音对话管线：音频输入 -> STT（语音转文本）->
LLM 推理 -> TTS（文本转语音）-> 音频输出。支持流式处理以降低
端到端延迟，并内置 VAD（语音活动检测）和用户打断机制。
同时提供 Native Audio 模式，支持 GPT-4o 风格的原生多模态音频推理，
目标延迟可低至 232ms。

# 核心接口

  - STTProvider / STTStream：语音转文本提供者及其流式会话
  - TTSProvider：文本转语音提供者，支持单次合成和流式合成
  - LLMHandler：语音场景下的 LLM 流式推理处理器
  - NativeAudioProvider：原生音频模型接口，支持单次和流式音频处理

# 主要能力

  - VoiceAgent：语音 Agent 核心，编排 STT/LLM/TTS 管线，管理
    Idle/Listening/Processing/Speaking/Interrupted 五态状态机
  - VoiceSession：单次语音会话，管理音频输入 channel、转录处理
    和语音合成输出，支持用户打断当前发言
  - NativeAudioReasoner：原生音频推理器，跟踪延迟指标（平均值、
    目标命中率），支持流式逐帧处理
  - VoiceMetrics / AudioMetrics：性能度量，包含会话数、延迟分布、
    打断次数和音频时长统计

# 与其他包协同

  - agent/streaming：通过 AudioStreamAdapter 实现底层音频传输
  - agent/hitl：高风险语音指令可触发人工审批中断
  - agent/skills：语音 Agent 可调用已注册技能作为后端能力
*/
package voice

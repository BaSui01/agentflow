// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 music 提供统一的 AI 音乐生成接入层，支持多服务商适配与统一请求模型。

# 概述

本包对上层业务暴露一致的音乐生成请求与响应结构，屏蔽不同服务商在
接口协议、鉴权方式和异步轮询机制上的差异。调用方只需构造
GenerateRequest 即可完成音乐创作，无需关心底层 API 细节。

典型使用场景：

  - 根据文本描述或歌词 Prompt 生成完整音乐。
  - 指定风格（pop、rock、jazz 等）与时长进行定制化生成。
  - 基于参考音频进行风格延续或续写。
  - 纯器乐模式生成无人声音轨。

# 核心接口

  - MusicProvider：统一的音乐生成 Provider 接口，包含 Generate 与 Name 方法。
  - GenerateRequest / GenerateResponse：标准化的请求与响应模型。
  - MusicData：单条音轨数据，包含 URL、Base64 音频、时长、标题与歌词。
  - MusicUsage：用量统计，包含生成曲目数与总时长。

# 主要能力

  - Suno 适配：通过 SunoProvider 接入 Suno API，支持异步任务轮询。
  - MiniMax 适配：通过 MiniMaxProvider 接入 MiniMax API，支持参考音频与音质设置。
  - 配置管理：每个 Provider 提供独立的 Config 结构与 Default 工厂函数。
*/
package music

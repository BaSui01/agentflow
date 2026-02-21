// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 streaming 提供实时双向流式交互能力。

# 概述

streaming 实现了基于 channel 的双向流通信框架，支持文本、音频、
视频及混合类型数据的实时传输。内置心跳检测、自动重连（指数退避）
和连接状态机管理，适用于对延迟敏感的 Agent 交互场景。

# 核心接口

  - StreamConnection：底层流式连接抽象（WebSocket、gRPC 等），定义
    ReadChunk / WriteChunk / Close / IsAlive 四个方法
  - StreamHandler：流数据处理回调，分别处理入站、出站数据和状态变更
  - AudioEncoder / AudioDecoder：音频编解码器接口，用于音频流适配

# 主要能力

  - BidirectionalStream：核心双向流，管理入站/出站 channel、序列号、
    心跳和自动重连，通过 connFactory 支持断线重建连接
  - StreamManager：多流管理器，统一创建、检索和关闭流实例
  - StreamSession：流会话统计，跟踪发送/接收的字节数和 chunk 数
  - WebSocketStreamConnection：将 nhooyr.io/websocket 适配为
    StreamConnection，写操作通过 mutex 保护并发安全
  - AudioStreamAdapter / TextStreamAdapter：类型化适配器，简化
    音频 PCM 和文本数据的收发
  - StreamReader / StreamWriter：将流包装为标准 io.Reader / io.Writer

# 与其他包协同

  - agent/voice：语音 Agent 通过 AudioStreamAdapter 进行实时音频传输
  - agent/hitl：流式交互中可注入人工中断节点
*/
package streaming

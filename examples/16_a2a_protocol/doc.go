// 版权所有 2024 AgentFlow Authors. 保留所有权利。
// 此源代码的使用由 MIT 许可规范，该许可可以
// 在 LICENSE 文件中找到。

/*
示例 16_a2a_protocol 演示了 AgentFlow 的 Agent-to-Agent（A2A）通信协议。

# 演示内容

本示例展示 A2A 协议的三个核心环节：

  - Agent Card：Agent 能力名片的创建与生成，包括 Capability 声明、
    Tool 定义、输入输出 Schema 以及元数据配置
  - A2A Message：标准化消息的创建、校验、序列化与解析，
    支持 Task/Result 消息类型及 Reply 链路追踪
  - Client/Server：HTTP 客户端与服务端的配置演示，
    涵盖 /.well-known/agent.json 发现端点、同步/异步消息收发、
    认证鉴权及超时控制

# 运行方式

	go run .
*/
package main

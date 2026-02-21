// 版权所有 2024 AgentFlow Authors. 保留所有权利。
// 此源代码的使用由 MIT 许可规范，该许可可以
// 在 LICENSE 文件中找到。

/*
示例 07_mid_priority_features 演示了 AgentFlow 的中优先级功能集合。

# 演示内容

本示例涵盖六项核心能力：

  - Hosted Tools：OpenAI SDK 风格的托管工具注册与调用，包括 WebSearch 等内置工具
  - Agent Handoff：Agent 间任务移交协议，支持能力声明与自动路由
  - Role-based Crews：CrewAI 风格的角色化团队协作，支持 Sequential 流程编排
  - Conversation Mode：AutoGen 风格的多 Agent 对话，支持 RoundRobin 轮询策略
  - Bidirectional Streaming：双向流式通信，支持 VAD 语音活动检测与低延迟缓冲
  - Tracing Integration：LangSmith 风格的可观测性追踪，覆盖 LLM 调用与 Tool 调用链路

# 运行方式

	go run .
*/
package main

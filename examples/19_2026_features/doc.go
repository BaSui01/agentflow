// 版权所有 2024 AgentFlow Authors. 保留所有权利。
// 此源代码的使用由 MIT 许可规范，该许可可以
// 在 LICENSE 文件中找到。

/*
示例 19_2026_features 演示了 AgentFlow 面向 2026 年的前沿特性。

# 演示内容

本示例展示五项实验性能力：

  - Layered Memory：分层记忆系统，包含 Episodic（情景记忆）、
    Working（工作记忆，带 TTL）和 Procedural（程序性记忆）三层
  - GraphRAG：基于知识图谱的检索增强生成，支持节点/边的构建与邻居查询
  - Agentic Browser：自主浏览器代理，支持动作序列执行、延迟控制与超时配置
  - Native Audio Reasoning：原生音频推理，支持低延迟语音处理与 VAD 检测
  - Shadow AI Detection：影子 AI 检测，支持域名识别与内容扫描，
    可发现未授权的 API Key 使用和外部 AI 服务调用

# 运行方式

	go run .
*/
package main

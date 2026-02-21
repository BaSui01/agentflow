// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 moderation 提供统一的内容审核服务抽象，用于检测文本和图像中的
违规内容，支持多种审核类别与置信度评分。

# 概述

本包定义了内容审核的核心接口与数据模型，屏蔽不同审核服务商在
API 协议和分类体系上的差异。当前内置 OpenAI Moderation API 适配，
支持纯文本与多模态（文本 + 图像）混合审核。

# 核心接口

  - ModerationProvider：审核提供者接口，包含 Name 与 Moderate
    两个方法。
  - ModerationRequest：审核请求，支持文本列表与 Base64 图像列表。
  - ModerationResponse：审核响应，包含提供者、模型与结果列表。
  - ModerationResult：单条输入的审核结果，包含标记状态、分类与评分。
  - ModerationCategory：布尔型分类标记（hate、harassment、
    self-harm、sexual、violence、illicit 等 11 个类别）。
  - ModerationScores：各类别的置信度评分（float64）。

# 主要能力

  - 文本审核：对文本列表进行违规检测，返回逐条标记与评分。
  - 多模态审核：同时审核文本与 Base64 编码图像。
  - 多类别检测：覆盖仇恨、骚扰、自残、色情、暴力、非法等类别。
  - 配置体系：OpenAIConfig 提供 API Key、BaseURL、Model 与
    Timeout 配置，DefaultOpenAIConfig 返回合理默认值。
*/
package moderation

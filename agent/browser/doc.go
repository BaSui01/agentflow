// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 browser 为智能体提供浏览器自动化与网页交互能力。

# 概述

browser 使智能体能够像人类一样操作网页：导航、点击、输入、
截图、提取内容。它同时支持传统的 CSS Selector 驱动模式和
基于视觉模型的 Vision-Action Loop 模式，覆盖从结构化页面
到动态渲染页面的自动化需求。

# 核心接口

  - Browser：浏览器自动化的顶层抽象，定义 Execute / GetState /
    Close 三个核心方法
  - BrowserDriver：底层浏览器控制接口，提供 Navigate / Screenshot /
    Click / Type / Scroll / GetURL 等原子操作
  - VisionModel：视觉分析接口，负责截图分析（Analyze）与
    动作规划（PlanActions），驱动 Vision-Action Loop
  - BrowserFactory：浏览器实例工厂接口，用于池化管理

# 主要能力

  - 命令式操作：通过 BrowserCommand 发送 navigate / click / type /
    scroll / screenshot / extract / wait 等 12 种动作类型
  - 会话管理：BrowserSession 封装单次浏览器会话，自动记录命令历史，
    BrowserTool 管理多会话的创建与销毁
  - Vision-Action Loop：AgenticBrowser 结合 VisionModel 实现
    "截图 → 分析 → 规划 → 执行"的自主浏览循环，
    支持目标检测、最大动作数限制与失败重试
  - 实例池化：BrowserPool 提供浏览器实例的预创建、按需获取、
    归还与统一关闭，控制并发资源消耗
  - LLM 视觉适配：LLMVisionAdapter 将任意 LLMVisionProvider
    适配为 VisionModel 接口，桥接 LLM 多模态能力

# 内置实现

ChromeDPDriver 与 ChromeDPBrowser 基于 chromedp 库提供
完整的 Headless Chrome 驱动实现，支持代理、自定义 UserAgent、
错误时自动截图等特性。ChromeDPBrowserFactory 实现 BrowserFactory
接口，可直接接入 BrowserPool。

# 与其他包协同

browser 作为 Agent 的工具能力接入，通常由 agent/tools 注册后
在任务执行中按需调用。AgenticBrowser 可与 agent/conversation
配合，实现多轮对话驱动的网页操作流程。
*/
package browser

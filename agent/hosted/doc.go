// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 hosted 提供由平台托管的工具注册与远程服务接入能力。

# 概述

hosted 解决的核心问题是：Agent 在执行过程中需要调用外部能力（如网络搜索、
文件检索、代码执行等），这些能力由平台侧统一托管而非 Agent 自行实现。
本包提供了标准化的工具接口、注册中心与内置工具实现，使 Agent 能够以
统一方式发现和调用托管服务。

# 核心接口

本包围绕以下类型展开：

  - HostedTool：托管工具接口，定义 Type / Name / Schema / Execute 标准契约
  - ToolRegistry：工具注册中心，线程安全地管理工具的注册、查找与 Schema 导出
  - HostedToolType：工具类型枚举（web_search / file_search / code_execution / retrieval）

# 内置工具

  - WebSearchTool：网络搜索工具，通过 HTTP API 执行实时搜索并返回结构化结果
  - FileSearchTool：文件搜索工具，基于 FileSearchStore 接口执行语义检索

每个工具均实现 HostedTool 接口，支持 JSON Schema 参数描述与 context 感知执行。

# 使用方式

  1. 创建 ToolRegistry 并注册所需工具
  2. 通过 Registry.GetSchemas() 将工具能力暴露给 LLM
  3. LLM 返回工具调用时，通过 Registry.Get(name) 查找并执行对应工具

# 扩展方式

实现 HostedTool 接口即可接入自定义托管工具，例如数据库查询、
内部 API 网关调用或第三方 SaaS 服务集成。
*/
package hosted

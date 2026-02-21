// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 deployment 提供智能体的部署管理与运行环境适配能力。

# 概述

本包用于解决"如何将智能体部署到不同运行环境并管理其生命周期"的问题。
通过 Provider 抽象屏蔽底层平台差异，提供统一的部署、扩缩容、
健康检查与清理接口。

# 核心模型

  - Deployment：部署实例，记录目标平台、状态、副本数、资源配额与端点。
  - DeploymentProvider：部署后端接口，定义 Deploy / Update / Delete /
    Scale / GetStatus / GetLogs 六项操作。
  - Deployer：部署管理器，注册多个 Provider 并路由部署请求。
  - DeploymentConfig：镜像、端口、环境变量、Secret 引用与自动扩缩策略。
  - ResourceConfig：CPU / Memory / GPU 资源请求与限制。

# 主要能力

  - 多目标部署：支持 Kubernetes、Cloud Run、Lambda 与本地环境。
  - 生命周期管理：创建、更新、删除、扩缩容的完整流程。
  - 自动扩缩容：基于 CPU、内存或 QPS 指标的 AutoScale 配置。
  - 健康检查：可配置路径、间隔、超时与失败阈值。
  - Manifest 导出：将部署配置导出为 Kubernetes Deployment YAML/JSON。

# 与 agent 包协同

deployment 位于 agent 生命周期的末端：当智能体构建完成后，
通过 Deployer 将其发布到目标环境，并持续监控运行状态与资源使用，
实现从开发到生产的完整闭环。
*/
package deployment

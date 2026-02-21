// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 execution 提供安全隔离的代码沙箱执行能力。

# 概述

本包用于解决"智能体生成的代码如何在受控环境中安全执行"的问题。
通过 Docker 容器或本地进程两种后端，在资源限制、网络隔离与
代码验证的多重防护下执行用户代码，并收集执行结果。

# 核心接口

  - ExecutionBackend：执行后端抽象，定义 Execute / Cleanup / Name
    三项操作，由 DockerBackend 和 ProcessBackend 分别实现。
  - SandboxExecutor：沙箱执行器，封装后端调用、请求校验、
    超时控制与输出截断，并维护执行统计。
  - CodeValidator：执行前代码安全检查，按语言匹配危险模式黑名单。
  - SandboxTool：将沙箱执行器包装为 JSON 输入/输出的智能体工具。

# 主要能力

  - 多语言支持：Python、JavaScript、TypeScript、Go、Rust、Bash。
  - Docker 隔离：内存/CPU 限制、网络禁用、只读文件系统、权限降级。
  - 进程后端：轻量级本地执行，需显式启用，适用于受信环境。
  - 代码验证：针对各语言的危险 API 模式进行预执行拦截。
  - 资源治理：超时自动终止、输出大小截断、容器自动清理。
  - 执行统计：跟踪总执行数、成功/失败/超时计数与累计耗时。

# 与 agent 包协同

execution 作为智能体工具链的底层执行层：agent 通过 SandboxTool
将代码执行请求提交到沙箱，获取标准化的 ExecutionResult 后
继续后续推理，确保代码执行不会影响宿主环境安全。
*/
package execution

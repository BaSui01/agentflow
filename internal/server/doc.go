// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 server 提供 HTTP/HTTPS 服务器生命周期管理，支持非阻塞启动、
优雅关闭与系统信号监听。

# 概述

本包通过 Manager 封装 net/http.Server，统一管理监听、服务、
关闭与错误传播流程。支持 HTTP 与 TLS 两种启动模式，内置
SIGINT/SIGTERM 信号处理，适用于生产环境的优雅停机需求。

# 核心类型

  - Manager：HTTP 服务器管理器，持有 http.Server、net.Listener
    与异步错误通道，提供 Start/StartTLS/Shutdown/WaitForShutdown
    等生命周期方法。
  - Config：服务器配置，包含监听地址、读写超时、空闲超时、
    最大请求头大小与优雅关闭超时。

# 主要能力

  - 非阻塞启动：Start/StartTLS 在后台 goroutine 中运行服务，
    主线程不阻塞。
  - 优雅关闭：Shutdown 在配置的超时内完成请求排空与连接释放。
  - 信号监听：WaitForShutdown 监听 SIGINT/SIGTERM，收到信号后
    自动触发优雅关闭流程。
  - 错误传播：Errors() 返回异步错误通道，供调用方监控服务异常。
  - TLS 支持：默认配置 TLS，通过 StartTLS 指定证书与密钥文件。
  - 状态查询：IsRunning/Addr 提供运行状态与监听地址查询。
*/
package server

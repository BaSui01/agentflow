// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 streaming 提供面向 LLM 流式输出场景的高性能数据传输原语，
包括零拷贝缓冲、背压流控、速率限制与流多路复用。

# 概述

在大语言模型的流式响应中，token 以高频增量方式到达，对缓冲效率和
流量控制提出了较高要求。本包围绕这两个核心问题提供一组可组合的构建块：

  - 零拷贝缓冲：减少内存分配与数据复制开销。
  - 背压流控：在生产者速度超过消费者时自动施加反压。
  - 速率限制：基于令牌桶算法控制 token 消费速率。
  - 流多路复用：将单一源流扇出到多个消费者。

# 核心接口

  - ZeroCopyBuffer — 可增长的零拷贝读写缓冲，支持并发安全访问。
  - RingBuffer — 无锁环形缓冲，适用于单生产者/单消费者场景。
  - ChunkReader — 对连续字节切片进行零拷贝分块读取。
  - StringView — 基于 unsafe 的零拷贝 []byte→string 视图。
  - BackpressureStream — 带高/低水位线的背压流，支持 Block、DropOldest、
    DropNewest、Error 四种丢弃策略。
  - StreamMultiplexer — 将一个 BackpressureStream 扇出给多个消费者。
  - RateLimiter — 令牌桶速率限制器，支持阻塞等待。

# 主要能力

  - 零拷贝：BytesToString / StringToBytes 利用 unsafe 实现零分配转换。
  - 背压控制：通过 HighWaterMark / LowWaterMark 自动暂停与恢复生产者。
  - 可观测：BackpressureStream.Stats() 暴露 produced/consumed/dropped 等指标。
  - 扇出：StreamMultiplexer 支持运行时动态添加消费者。
*/
package streaming

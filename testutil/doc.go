// Copyright 2026 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license.

/*
Package testutil 提供 AgentFlow 测试的共享工具和辅助函数。

# 概述

testutil 包为整个项目的单元测试与基准测试提供统一的辅助能力，
避免各包重复实现相似的测试基础设施。所有测试应优先使用此包
中的工具函数和 Mock 实现。

# 核心能力

  - 上下文辅助: TestContext / TestContextWithTimeout / CancelledContext，
    自动注册 Cleanup 防止泄漏
  - 断言工具: AssertMessagesEqual / AssertToolCallsEqual / AssertJSONEqual /
    AssertNoError / AssertError / AssertContains 等
  - 异步断言: AssertEventuallyTrue / AssertEventuallyEqual，
    支持超时轮询等待条件满足
  - 数据工具: MustJSON / MustParseJSON / CopyMessages / CopyToolCalls，
    简化测试数据构造与深拷贝
  - 流式辅助: CollectStreamChunks / CollectStreamContent /
    SendChunksToChannel，用于 LLM 流式响应测试
  - 基准辅助: BenchmarkHelper 封装 testing.B 常用操作

# 子包

  - testutil/mocks: Mock 实现，包括 MockProvider（LLM Provider）、
    MockMemoryManager（记忆管理器）、MockToolManager（工具管理器），
    均支持 Builder 模式与错误注入
  - testutil/fixtures: 测试数据工厂，提供预置 Agent 配置、
    ChatResponse、StreamChunk、ToolSchema、对话历史等样例

# 使用示例

	ctx := testutil.TestContext(t)
	provider := mocks.NewMockProvider().WithResponse("hello")
	resp, err := provider.Completion(ctx, req)
	testutil.AssertNoError(t, err)
*/
package testutil

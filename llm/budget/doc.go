// 版权所有 2024 AgentFlow Authors. 版权所有。
// 此源代码的使用由 MIT 许可规范,该许可可以是
// 在LICENSE文件中找到。

/*
包 budget 提供 Token 预算管理与成本控制能力，通过多时间窗口
限流与告警机制防止 LLM 调用超支。

# 概述

LLM 调用按 Token 计费，不加控制容易产生意外高额费用。
本包通过 TokenBudgetManager 在分钟、小时、天三个时间窗口
同时跟踪 Token 用量与费用，并在接近阈值时触发告警或自动限流。

# 核心接口

  - TokenBudgetManager：核心预算管理器，负责用量记录、限额检查与告警触发。
  - BudgetConfig：配置各时间窗口的 Token 上限、费用上限、告警阈值与限流策略。
  - AlertHandler：告警回调函数，当用量超过阈值时被调用。

# 主要能力

  - 多窗口限额：支持 per-request、per-minute、per-hour、per-day 四级 Token 限制。
  - 费用控制：支持 per-request 与 per-day 两级费用上限。
  - 自动限流：当分钟窗口触顶时自动 throttle，延迟后续请求。
  - 阈值告警：用量达到可配置百分比时触发 Alert，支持注册多个 handler。
  - 线程安全：使用 atomic 操作与 RWMutex 保证高并发下的正确性。
  - 窗口自动重置：时间窗口到期后计数器自动归零。

# 使用方式

	cfg := budget.DefaultBudgetConfig()
	mgr := budget.NewTokenBudgetManager(cfg, logger)
	mgr.OnAlert(func(a budget.Alert) { log.Println(a.Message) })

	if err := mgr.CheckBudget(ctx, 5000, 0.05); err != nil {
	    // 超出预算，拒绝请求
	}
	mgr.RecordUsage(budget.UsageRecord{Tokens: 4800, Cost: 0.048, Model: "gpt-4"})
*/
package budget

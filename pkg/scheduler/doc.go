// Package scheduler provides a cron-style scheduled task runner integrated
// with the AgentFlow service lifecycle (pkg/service).
//
// Usage:
//
//	sch := scheduler.New(scheduler.Config{
//	    Logger: logger,
//	    Tasks: []scheduler.Task{
//	        {Name: "daily-report", CronExpr: "0 9 * * *", AgentID: "report-agent", Prompt: "生成今日报告"},
//	        {Name: "health-check", CronExpr: "*/5 * * * *", AgentID: "monitor-agent", Prompt: "检查系统健康状态"},
//	    },
//	})
//	registry.Register(sch, service.ServiceInfo{Name: "scheduler", Priority: 100})
package scheduler
# P0 优化提示词

## P0-1: API 安全中间件

### 需求背景
cmd/agentflow/main.go 的 startHTTPServer（第 380 行）直接用 http.NewServeMux() 裸挂 handler，没有任何中间件。生产环境必须有认证、限流、CORS、日志、panic 恢复。

### 需要修改的文件

#### 新建文件：cmd/agentflow/middleware.go

```go
package main

import (
    "fmt"
    "net/http"
    "strings"
    "sync"
    "time"

    "go.uber.org/zap"
    "golang.org/x/time/rate"
)

// Middleware 类型定义
type Middleware func(http.Handler) http.Handler

// Chain 将多个中间件串联
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
    for i := len(middlewares) - 1; i >= 0; i-- {
        h = middlewares[i](h)
    }
    return h
}

// Recovery panic 恢复中间件
func Recovery(logger *zap.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if err := recover(); err != nil {
                    logger.Error("panic recovered", zap.Any("error", err), zap.String("path", r.URL.Path))
                    http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}

// RequestLogger 请求日志中间件
func RequestLogger(logger *zap.Logger) Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
            next.ServeHTTP(rw, r)
            logger.Info("request",
                zap.String("method", r.Method),
                zap.String("path", r.URL.Path),
                zap.Int("status", rw.statusCode),
                zap.Duration("duration", time.Since(start)),
                zap.String("remote_addr", r.RemoteAddr),
            )
        })
    }
}

type responseWriter struct {
    http.ResponseWriter
    statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
    rw.statusCode = code
    rw.ResponseWriter.WriteHeader(code)
}

// APIKeyAuth API Key 认证中间件
// skipPaths 中的路径不需要认证（如 /health, /healthz, /ready, /readyz, /version, /metrics）
func APIKeyAuth(validKeys []string, skipPaths []string, logger *zap.Logger) Middleware {
    keySet := make(map[string]struct{}, len(validKeys))
    for _, k := range validKeys {
        keySet[k] = struct{}{}
    }
    skipSet := make(map[string]struct{}, len(skipPaths))
    for _, p := range skipPaths {
        skipSet[p] = struct{}{}
    }
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if _, skip := skipSet[r.URL.Path]; skip {
                next.ServeHTTP(w, r)
                return
            }
            key := r.Header.Get("X-API-Key")
            if key == "" {
                key = r.URL.Query().Get("api_key")
            }
            if _, ok := keySet[key]; !ok {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusUnauthorized)
                fmt.Fprint(w, `{"error":"unauthorized","message":"invalid or missing API key"}`)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
// RateLimiter 基于 IP 的请求限流中间件
func RateLimiter(rps float64, burst int, logger *zap.Logger) Middleware {
    type visitor struct {
        limiter  *rate.Limiter
        lastSeen time.Time
    }
    var (
        mu       sync.Mutex
        visitors = make(map[string]*visitor)
    )
    // 后台清理过期 visitor
    go func() {
        for {
            time.Sleep(time.Minute)
            mu.Lock()
            for ip, v := range visitors {
                if time.Since(v.lastSeen) > 3*time.Minute {
                    delete(visitors, ip)
                }
            }
            mu.Unlock()
        }
    }()
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ip := strings.Split(r.RemoteAddr, ":")[0]
            mu.Lock()
            v, exists := visitors[ip]
            if !exists {
                v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
                visitors[ip] = v
            }
            v.lastSeen = time.Now()
            mu.Unlock()
            if !v.limiter.Allow() {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(http.StatusTooManyRequests)
                fmt.Fprint(w, `{"error":"rate_limit_exceeded","message":"too many requests"}`)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
// CORS 跨域中间件
func CORS(allowedOrigins []string) Middleware {
    originSet := make(map[string]struct{}, len(allowedOrigins))
    for _, o := range allowedOrigins {
        originSet[o] = struct{}{}
    }
    allowAll := len(allowedOrigins) == 0
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            origin := r.Header.Get("Origin")
            if allowAll || origin == "" {
                w.Header().Set("Access-Control-Allow-Origin", "*")
            } else if _, ok := originSet[origin]; ok {
                w.Header().Set("Access-Control-Allow-Origin", origin)
            }
            w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
            w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
            w.Header().Set("Access-Control-Max-Age", "86400")
            if r.Method == http.MethodOptions {
                w.WriteHeader(http.StatusNoContent)
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

#### 修改文件：cmd/agentflow/main.go

在 startHTTPServer 方法中，将 `s.httpServer.Handler = mux` 改为用中间件链包装：

```go
func (s *Server) startHTTPServer() error {
    mux := http.NewServeMux()
    // ... 注册路由不变 ...

    // 构建中间件链
    skipAuthPaths := []string{"/health", "/healthz", "/ready", "/readyz", "/version", "/metrics"}
    handler := Chain(mux,
        Recovery(s.logger),
        RequestLogger(s.logger),
        CORS(s.cfg.Server.CORSAllowedOrigins),
        RateLimiter(float64(s.cfg.Server.RateLimitRPS), s.cfg.Server.RateLimitBurst, s.logger),
        APIKeyAuth(s.cfg.Server.APIKeys, skipAuthPaths, s.logger),
    )

    s.httpServer = &http.Server{
        Addr:         fmt.Sprintf(":%d", s.cfg.Server.HTTPPort),
        Handler:      handler, // 用中间件链包装！
        ReadTimeout:  s.cfg.Server.ReadTimeout,
        WriteTimeout: s.cfg.Server.WriteTimeout,
    }
    // ...
}
```

#### 修改文件：config 包添加配置字段

在 Server 配置结构体中添加：
```go
CORSAllowedOrigins []string `yaml:"cors_allowed_origins" json:"cors_allowed_origins,omitempty"`
APIKeys            []string `yaml:"api_keys" json:"api_keys,omitempty"`
RateLimitRPS       int      `yaml:"rate_limit_rps" json:"rate_limit_rps,omitempty"`     // 默认 100
RateLimitBurst     int      `yaml:"rate_limit_burst" json:"rate_limit_burst,omitempty"` // 默认 200
```

### 设计原则
- 向后兼容：APIKeys 为空时不启用认证
- 健康检查端点永远不需要认证
- 中间件顺序：Recovery → Logger → CORS → RateLimit → Auth

### 测试要求
- 测试认证中间件：有效 key、无效 key、跳过路径
- 测试限流中间件：正常请求、超限请求
- 测试 CORS：预检请求、允许的 origin、不允许的 origin
- 测试 Recovery：handler panic 后返回 500

---
## P0-2: 成本控制模块完善

### 需求背景
llm/tools/cost_control.go 的 DefaultCostController 有基础框架但关键逻辑不完整：CalculateCost 只按参数大小估算、周期重置未实现、GetCostReport/GetOptimizations 可能未实现。

### 需要修改的文件

#### 文件：llm/tools/cost_control.go

**改动 1 — 增强 CalculateCost（第 231 行附近）**

当前代码只是 `BaseCost + len(args)/100 * CostPerUnit`，需要改为支持按 token 计算：

```go
// TokenCounter 可选的 token 计数器接口
type TokenCounter interface {
    CountTokens(text string) (int, error)
}

func (cc *DefaultCostController) CalculateCost(toolName string, args json.RawMessage) (float64, error) {
    cc.mu.RLock()
    defer cc.mu.RUnlock()

    toolCost, ok := cc.toolCosts[toolName]
    if !ok {
        return 1.0, nil // 默认成本
    }

    cost := toolCost.BaseCost

    if toolCost.CostPerUnit > 0 && len(args) > 0 {
        switch toolCost.Unit {
        case CostUnitTokens:
            // 如果有 token 计数器，精确计算
            if cc.tokenCounter != nil {
                tokens, err := cc.tokenCounter.CountTokens(string(args))
                if err == nil {
                    cost += float64(tokens) * toolCost.CostPerUnit
                    break
                }
            }
            // fallback: 按字符数估算（1 token ≈ 4 字符）
            cost += float64(len(args)) / 4.0 * toolCost.CostPerUnit
        case CostUnitCredits, CostUnitDollars:
            cost += float64(len(args)) / 100.0 * toolCost.CostPerUnit
        }
    }

    return cost, nil
}
```
**改动 2 — 添加周期重置逻辑**

在 DefaultCostController 中添加：
```go
// resetUsageIfNeeded 检查并重置过期的预算周期
func (cc *DefaultCostController) resetUsageIfNeeded(budget *Budget) {
    key := cc.buildUsageKey(budget)
    resetKey := key + ":reset_at"

    lastReset, exists := cc.usageResetTimes[resetKey]
    if !exists {
        cc.usageResetTimes[resetKey] = time.Now()
        return
    }

    var shouldReset bool
    switch budget.Period {
    case BudgetPeriodHourly:
        shouldReset = time.Since(lastReset) >= time.Hour
    case BudgetPeriodDaily:
        shouldReset = time.Since(lastReset) >= 24*time.Hour
    case BudgetPeriodWeekly:
        shouldReset = time.Since(lastReset) >= 7*24*time.Hour
    case BudgetPeriodMonthly:
        shouldReset = time.Since(lastReset) >= 30*24*time.Hour
    case BudgetPeriodTotal:
        shouldReset = false
    }

    if shouldReset {
        cc.usage[key] = 0
        cc.usageResetTimes[resetKey] = time.Now()
        cc.logger.Info("budget usage reset", zap.String("budget_id", budget.ID), zap.String("period", string(budget.Period)))
    }
}
```

DefaultCostController 结构体添加字段：
```go
usageResetTimes map[string]time.Time
tokenCounter    TokenCounter
```
**改动 3 — 完善 CheckBudget 中的告警通知**

在 CheckBudget 的告警阈值检查后，调用 alertHandler：
```go
// 在 CheckBudget 中，告警阈值检查后添加：
if cc.alertHandler != nil {
    alert := &CostAlert{
        ID:         fmt.Sprintf("alert_%d", time.Now().UnixNano()),
        Timestamp:  time.Now(),
        Level:      alertLevel,
        BudgetID:   budget.ID,
        Message:    fmt.Sprintf("budget %s at %.1f%%", budget.Name, percentage),
        Current:    newUsage,
        Limit:      budget.Limit,
        Percentage: percentage,
    }
    go cc.alertHandler.HandleAlert(context.Background(), alert)
    result.Alert = alert
}
```

**改动 4 — 实现 GetCostReport**

```go
func (cc *DefaultCostController) GetCostReport(filter *CostReportFilter) (*CostReport, error) {
    cc.mu.RLock()
    defer cc.mu.RUnlock()

    report := &CostReport{
        ByTool:      make(map[string]float64),
        ByAgent:     make(map[string]float64),
        ByUser:      make(map[string]float64),
        ByDay:       make(map[string]float64),
        GeneratedAt: time.Now(),
    }

    for _, rec := range cc.records {
        // 应用过滤器
        if filter != nil {
            if filter.AgentID != "" && rec.AgentID != filter.AgentID { continue }
            if filter.UserID != "" && rec.UserID != filter.UserID { continue }
            if filter.ToolName != "" && rec.ToolName != filter.ToolName { continue }
            if filter.StartTime != nil && rec.Timestamp.Before(*filter.StartTime) { continue }
            if filter.EndTime != nil && rec.Timestamp.After(*filter.EndTime) { continue }
        }

        report.TotalCost += rec.Cost
        report.TotalCalls++
        report.ByTool[rec.ToolName] += rec.Cost
        report.ByAgent[rec.AgentID] += rec.Cost
        report.ByUser[rec.UserID] += rec.Cost
        day := rec.Timestamp.Format("2006-01-02")
        report.ByDay[day] += rec.Cost
    }

    if report.TotalCalls > 0 {
        report.AverageCost = report.TotalCost / float64(report.TotalCalls)
    }

    return report, nil
}
```
**改动 5 — 实现 GetOptimizations**

```go
func (cc *DefaultCostController) GetOptimizations(agentID, userID string) []*CostOptimization {
    cc.mu.RLock()
    defer cc.mu.RUnlock()

    var opts []*CostOptimization

    // 分析高频工具
    toolCalls := make(map[string]int)
    toolCosts := make(map[string]float64)
    for _, rec := range cc.records {
        if agentID != "" && rec.AgentID != agentID { continue }
        if userID != "" && rec.UserID != userID { continue }
        toolCalls[rec.ToolName]++
        toolCosts[rec.ToolName] += rec.Cost
    }

    for tool, count := range toolCalls {
        cost := toolCosts[tool]
        avgCost := cost / float64(count)
        // 高频高成本工具建议缓存
        if count > 100 && avgCost > 5.0 {
            opts = append(opts, &CostOptimization{
                Type:        "cache",
                Description: fmt.Sprintf("Tool '%s' called %d times with avg cost %.2f. Consider caching results.", tool, count, avgCost),
                Savings:     cost * 0.3,
                Priority:    1,
            })
        }
        // 低使用率高成本工具建议替换
        if count < 10 && cost > 100 {
            opts = append(opts, &CostOptimization{
                Type:        "replace",
                Description: fmt.Sprintf("Tool '%s' has high total cost (%.2f) with low usage (%d calls). Consider cheaper alternative.", tool, cost, count),
                Savings:     cost * 0.5,
                Priority:    2,
            })
        }
    }

    return opts
}
```

### 设计原则
- 向后兼容：TokenCounter 可选，为 nil 时 fallback 到字符估算
- 周期重置在 CheckBudget 中惰性触发，不需要后台 goroutine
- 告警通知异步发送（go cc.alertHandler.HandleAlert）

### 测试要求
- 测试 CalculateCost 的 token 计算和 fallback
- 测试周期重置逻辑
- 测试告警阈值触发
- 测试 GetCostReport 的过滤和聚合
- 测试 GetOptimizations 的建议生成

---
## P0-3: toolProvider function calling 校验

### 需求背景
刚添加的双模型能力中，ChatCompletion 对 toolProvider 是否支持 SupportsNativeFunctionCalling() 没有校验。

### 需要修改的文件

#### 文件：agent/base.go

**改动 — 修改 function calling 校验逻辑（第 463-465 行附近）**

当前代码：
```go
if len(req.Tools) > 0 && b.provider != nil && !b.provider.SupportsNativeFunctionCalling() {
    return nil, fmt.Errorf("provider %q does not support native function calling", b.provider.Name())
}
```

改为：
```go
if len(req.Tools) > 0 {
    // 确定实际用于工具调用的 provider
    effectiveToolProvider := b.provider
    if b.toolProvider != nil {
        effectiveToolProvider = b.toolProvider
    }
    if effectiveToolProvider != nil && !effectiveToolProvider.SupportsNativeFunctionCalling() {
        return nil, fmt.Errorf("provider %q does not support native function calling", effectiveToolProvider.Name())
    }
}
```

这样无论是用 provider 还是 toolProvider，都会在进入 ReAct 循环前校验。
#### 文件：agent/tool_provider_test.go 添加测试

```go
// TestToolProvider_FunctionCallingValidation 测试 toolProvider 不支持 function calling 时报错
func TestToolProvider_FunctionCallingValidation(t *testing.T) {
    logger := zap.NewNop()
    mainProvider := new(MockProvider)
    toolProvider := new(MockProvider)
    toolMgr := new(MockToolManager)

    config := Config{
        ID:    "test-dual-fc",
        Name:  "Dual FC Agent",
        Type:  TypeGeneric,
        Model: "claude-sonnet-4-6",
        Tools: []string{"search"},
    }

    agent := NewBaseAgent(config, mainProvider, nil, toolMgr, nil, logger)
    agent.SetToolProvider(toolProvider)

    // toolProvider 不支持 function calling
    toolProvider.On("SupportsNativeFunctionCalling").Return(false)
    toolProvider.On("Name").Return("cheap-model")
    toolMgr.On("GetAllowedTools", "test-dual-fc").Return([]llm.ToolSchema{
        {Name: "search", Description: "search tool"},
    })

    _, err := agent.ChatCompletion(context.Background(), []llm.Message{
        {Role: llm.RoleUser, Content: "search something"},
    })

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "does not support native function calling")
    assert.Contains(t, err.Error(), "cheap-model")
}
```

### 设计原则
- 校验逻辑统一：不再分别校验 provider 和 toolProvider，而是校验"实际会被用于工具调用的那个"
- 错误信息明确：报错时显示实际 provider 的名字

### 测试要求
- toolProvider 不支持 FC 时应报错
- toolProvider 支持 FC 时正常执行
- toolProvider 为 nil 时退化校验 provider

---
## P2-11: ReAct MaxIterations 可配置

### 需求背景
MaxIterations: 10 硬编码在 ChatCompletion 的两处 NewReActExecutor 调用中，不同场景需要不同迭代次数。

### 需要修改的文件

#### 文件：agent/base.go

**改动 A — Config 结构体（第 96 行附近）添加字段：**
```go
MaxReActIterations int `json:"max_react_iterations,omitempty"` // ReAct 最大迭代次数，默认 10
```

**改动 B — ChatCompletion 中两处 ReActConfig 使用配置值：**

添加辅助方法：
```go
func (b *BaseAgent) maxReActIterations() int {
    if b.config.MaxReActIterations > 0 {
        return b.config.MaxReActIterations
    }
    return 10 // 默认值
}
```

两处 `MaxIterations: 10` 改为 `MaxIterations: b.maxReActIterations()`

#### 文件：agent/builder.go

添加 Builder 方法：
```go
func (b *AgentBuilder) WithMaxReActIterations(n int) *AgentBuilder {
    if n > 0 {
        b.config.MaxReActIterations = n
    }
    return b
}
```

### 测试要求
- 默认值为 10
- 设置为 5 时生效
- 设置为 0 时使用默认值
- Builder 链式调用正确

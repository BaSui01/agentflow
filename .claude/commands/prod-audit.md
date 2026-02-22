# 生产就绪度多智能体并行审计

启动 8 个并行审计智能体，对项目进行全面的生产就绪度评估。

* **行动先于提问**（先审计，再汇总，不问元问题）
* **并行最大化**（所有维度同时启动，不串行等待）
* **发现驱动修复**（审计结果直接产出 P0/P1/P2 修复清单）
* **可复用可增量**（支持按维度选择，支持增量审计）

---

## 参数

- `$ARGUMENTS` — 可选，指定审计范围。示例：
  - 空（默认）：执行全部 8 个维度
  - `infra,api`：仅审计基础设施和 API 一致性
  - `security`：仅审计安全维度
  - `all-fix`：审计 + 自动修复（启动 implement agent 修复 P0 问题）
  - 可用维度标签：`infra`, `api`, `arch`, `contract`, `test`, `entry`, `security`, `perf`

---

## 核心原则（不可妥协）

1. **行动先于提问**
   不要问"要审计哪些维度？"或"项目路径在哪？"。从工作目录和 git 状态推导一切。

2. **并行启动，不串行**
   所有审计 agent 必须在同一条消息中并行发送。不要一个一个启动。

3. **具体到文件和行号**
   每个发现必须引用具体文件路径和行号。"可能有问题"不是有效发现。

4. **评分基于事实**
   评分必须基于检查项的通过/失败数量，不是主观印象。

5. **修复建议可执行**
   每个问题必须附带具体的修复方案（改哪个文件、怎么改），不是泛泛建议。

6. **不重复已知信息**
   如果项目有 `.trellis/spec/` 规范，先读取，避免审计已记录的已知问题。

---

## 执行流程

### 步骤 0：自动上下文收集（在启动 agent 之前）

在问任何问题之前，自己收集上下文：

```bash
# 获取项目状态
python3 ./.trellis/scripts/get_context.py 2>/dev/null || true

# 读取项目规范（如果存在）
cat .trellis/spec/backend/index.md 2>/dev/null || true
cat .trellis/spec/backend/quality-guidelines.md 2>/dev/null || true
cat .trellis/spec/backend/error-handling.md 2>/dev/null || true
```

从上下文中提取：
- 项目语言和框架
- 已有的质量规范和已知问题
- 最近的提交历史（判断活跃模块）

### 步骤 1：确定审计范围

| 输入 | 行为 |
|------|------|
| `$ARGUMENTS` 为空 | 启动全部 8 个维度 |
| `$ARGUMENTS` 含维度标签 | 仅启动指定维度 |
| `$ARGUMENTS` 含 `all-fix` | 全部审计 + 自动修复 P0 |

### 步骤 2：创建 Team 并启动并行审计

使用 Agent Team 模式：

1. `TeamCreate` — 创建审计团队
2. `TaskCreate` × N — 为每个维度创建任务
3. `Task` × N — **在同一条消息中**并行启动所有 research agent

```
Task(
  subagent_type: "research",
  model: "sonnet",
  name: "audit-{dimension-label}",
  team_name: "{team-name}",
  run_in_background: true,
  prompt: "{维度 prompt}\n\n项目根目录: {cwd}\n\n请开始审计。"
)
```

> 关键：所有 Task 调用必须在同一条消息中发送，实现真正并行。

### 步骤 3：等待结果并汇总

- 每个 agent 完成后通过 SendMessage 发送报告
- 收齐所有报告后，汇总为总报告
- Shutdown 所有 agent → TeamDelete

### 步骤 4：修复建议（或自动修复）

汇总报告后：
- 列出 P0/P1/P2 问题清单，每个问题附带具体修复方案
- 如果 `$ARGUMENTS` 含 `all-fix`，为每个 P0 问题启动 implement agent 自动修复

---

## 审计维度定义

每个审计 agent 收到的 prompt 必须包含：
1. 角色定义和维度名称
2. 具体检查项列表（带审计方法）
3. 输出格式要求
4. 项目根目录路径

### 通用输出格式（所有维度共用）

```
对每个检查项给出：
- 状态：✅ 已实现 | ⚠️ 部分实现 | ❌ 缺失
- 文件引用：具体文件路径:行号
- 发现的问题（如有）：描述 + 严重程度（P0/P1/P2）
- 修复建议（如有）：具体改哪个文件、怎么改

最后给出：
- 维度总评分（1-10）
- 问题清单（按 P0 > P1 > P2 排序）

完成后用 SendMessage 将完整报告发送给 team-lead。
用 TaskUpdate 将任务标记为 completed。
```

---

### 维度 1：生产基础设施 (`infra`)

```
你是生产基础设施审计专家。

## 检查项
1. 健康检查端点（liveness /healthz, readiness /readyz, startup 三种探针）
2. 可观测性（OpenTelemetry traces + metrics + logs 三支柱集成）
3. Prometheus metrics 覆盖率（HTTP/LLM/Agent/Cache/DB 五大维度）
4. 数据库迁移机制（up/down 配对、回滚能力、dirty 状态修复）
5. 配置热重载（并发安全性、TOCTOU 防护、回滚机制、版本历史）
6. TLS 配置（最低版本、密码套件、证书管理）
7. Docker/Helm 部署配置完整性（多阶段构建、非 root、探针、HPA、PDB）
8. 限流/熔断机制（全局 + 租户级限流、三态熔断器）

## 审计方法
- 搜索 /health, /healthz, /ready, /readyz 端点实现
- 检查 OpenTelemetry SDK 初始化（TracerProvider, MeterProvider）
- 检查 Prometheus metrics 注册（Counter, Histogram, Gauge）
- 搜索 migration, migrate, golang-migrate 相关代码
- 检查 config hot reload 的锁机制和回滚逻辑
- 搜索 tls.Config, MinVersion, CipherSuites
- 检查 Dockerfile 的构建阶段、用户权限、HEALTHCHECK
- 检查 Helm values.yaml 的探针、安全上下文、资源限制
- 检查 rate limiter 的算法（令牌桶/滑动窗口）和作用域
- 检查 circuit breaker 的状态机（Closed/Open/HalfOpen）
```

### 维度 2：API 一致性 (`api`)

```
你是 API 一致性审计专家。

## 检查项
1. OpenAPI spec 与实际 handler 实现是否一致（端点数量、路径、方法）
2. 请求/响应类型定义与实际序列化是否匹配（字段名、类型、required）
3. 错误处理是否统一使用 Response 信封格式（所有层，含中间件）
4. 中间件链完整性（认证、限流、CORS、日志、metrics、tracing、recovery）
5. 路由注册与 OpenAPI paths 是否一一对应
6. HTTP 方法约束是否正确（GET 无 body、POST/PUT 有 Content-Type 验证）
7. Content-Type 验证是否一致
8. 条件路由注册是否在 OpenAPI 中有说明

## 审计方法
- 读取 OpenAPI spec 文件，列出所有 path + method
- 读取路由注册代码，列出所有 HandleFunc 调用
- 逐一对比，标记差异
- 读取 Go 类型定义的 json tag，对比 OpenAPI schema 的 properties
- 检查所有 handler 和中间件的错误响应格式
- 检查中间件注册顺序
```

### 维度 3：架构分层 (`arch`)

```
你是架构分层审计专家。

## 检查项
1. 包依赖方向（上层依赖下层，不反向；types 不依赖业务包）
2. 接口边界清晰度（每层通过接口通信，不直接依赖实现）
3. 循环依赖检测（go build 是否通过）
4. 分层违规（handler 直接访问 DB、cmd 包含业务逻辑等）
5. types 包纯净性（仅标准库依赖，无项目内 import）
6. internal 包使用是否合理（是否被不应引用的外部包引用）
7. 适配器/桥接模式使用是否正确

## 审计方法
- 读取 types/ 下所有文件的 import 语句
- 搜索 internal/ 包被哪些外部包引用
- 分析 agent/ → llm/ 的依赖方向
- 检查 cmd/ 包是否只做组装（不含业务逻辑）
- 运行 go build ./... 验证无循环依赖
```

### 维度 4：接口契约 (`contract`)

```
你是接口契约审计专家。

## 检查项
1. 核心接口（Agent, Provider, Executor, Tokenizer）实现完整性
2. Agent 接口各实现是否方法签名一致
3. Provider 接口各实现是否完整（抽查 3+ 个 provider）
4. 错误类型是否统一（types.Error vs agent.Error vs api.ErrorInfo 的关系）
5. Message/ToolSchema 等核心类型跨包一致性（是否有重复定义、字段丢失）
6. 类型转换函数是否完整（所有字段都被复制）
7. 接口方法的 context.Context 使用是否一致

## 审计方法
- 搜索所有 interface 定义，列出方法签名
- 对每个接口搜索实现者，检查方法是否完整
- 对比不同包中同名类型的字段定义
- 检查类型转换函数是否遗漏字段
- 搜索 type alias 和 type re-export
```

### 维度 5：测试质量 (`test`)

```
你是测试质量审计专家。

## 检查项
1. 核心包单元测试覆盖（agent, llm, workflow, rag, config, types）
2. handler 层测试（HTTP 请求/响应完整链路）
3. 集成测试（跨包交互）
4. 端到端测试（如有，检查覆盖的用户路径）
5. 基准测试覆盖（关键路径是否有 Benchmark）
6. 竞态测试（-race flag、*_race_test.go）
7. 契约测试（接口实现一致性、OpenAPI 一致性）
8. CI/CD 配置（测试是否在 CI 中运行、是否有包被排除）
9. 代码质量工具（golangci-lint 配置、govulncheck）
10. 覆盖率阈值是否合理

## 审计方法
- 搜索 *_test.go 文件分布，统计每个包的测试文件数
- 搜索 Benchmark* 函数
- 搜索 *_race_test.go 文件
- 检查 CI 配置中的 EXCLUDED_PKGS 或类似排除逻辑
- 检查 .golangci.yml 启用的 linter 数量和配置
- 检查 Makefile 中的覆盖率阈值
```

### 维度 6：入口一致性 (`entry`)

```
你是入口一致性审计专家。

## 检查项
1. CLI 入口完整性（命令注册、子命令、help 文本）
2. 根包导出一致性（公开 API 是否有二义性）
3. 配置加载流程（默认值 → 文件 → 环境变量 → 命令行 优先级）
4. 启动顺序（依赖初始化顺序是否正确，是否有未初始化就使用的风险）
5. 关闭顺序（资源释放顺序是否正确：服务器 → 遥测 → 数据库）
6. 版本信息注入（ldflags -X 变量与接收变量是否匹配）
7. 信号处理（SIGTERM/SIGINT graceful shutdown + 超时机制）
8. 配置中是否有未使用的字段（预留字段应有注释）

## 审计方法
- 读取 cmd/ 目录下的入口文件，列出所有命令
- 检查根包和 quick 包的导出是否一致
- 读取配置加载代码，验证优先级链
- 读取 Shutdown() 方法，验证关闭顺序
- 检查 Makefile 中的 ldflags 与代码中的 var 声明
- 搜索 signal.Notify 调用
```

### 维度 7：安全审计 (`security`)

```
你是安全审计专家。

## 检查项
1. CORS 配置安全性（是否默认允许 *，是否限制 methods/headers）
2. API Key 传输方式（仅 Header，不接受 Query String）
3. JWT 配置安全性（算法白名单、密钥强度、过期时间、刷新机制）
4. 输入验证完整性（路径参数正则、查询参数类型、请求体 JSON schema）
5. SQL 注入防护（参数化查询、ORM 使用、无 raw SQL 拼接）
6. 敏感信息泄露（日志中不打印 API Key/密码/token，配置脱敏）
7. 安全响应头（X-Frame-Options, X-Content-Type-Options, CSP, HSTS）
8. 依赖漏洞（go.sum 中的已知漏洞，govulncheck 结果）
9. 文件上传/路径遍历防护（如适用）
10. Rate limiting 是否覆盖认证端点（防暴力破解）

## 审计方法
- 检查 CORS 中间件的 AllowOrigin 配置
- 搜索 r.URL.Query().Get("api_key") 或类似的 query string API key
- 检查 JWT 中间件的 SigningMethod 白名单
- 搜索 r.PathValue(), r.URL.Query() 后是否有验证逻辑
- 搜索 db.Raw(), db.Exec() 中是否有字符串拼接
- 搜索 logger 调用中是否包含 "key", "password", "token", "secret" 字段
- 检查安全响应头中间件
- 如果有 govulncheck，运行并报告结果
```

### 维度 8：性能基线 (`perf`)

```
你是性能审计专家。

## 检查项
1. 基准测试覆盖（关键路径是否有 Benchmark：内存操作、路由、序列化）
2. 内存分配热点（循环内 make/append/new、大对象复制）
3. goroutine 泄漏检测（启动的 goroutine 是否都有退出机制）
4. 连接池配置（DB SetMaxOpenConns/SetMaxIdleConns、HTTP Transport MaxIdleConns）
5. 缓存策略（是否有适当的缓存层、过期/淘汰机制、缓存穿透防护）
6. JSON 序列化效率（encoding/json vs 更高效的库、是否有不必要的序列化）
7. sync.Pool 使用（高频分配对象是否池化：buffer、encoder）
8. context 超时传播（所有外部调用是否都有超时、是否正确传播 parent context）
9. Prometheus 指标基数（label 值是否有限、是否有高基数风险）
10. 批处理能力（是否支持批量操作减少 round-trip）

## 审计方法
- 搜索 Benchmark* 函数，统计覆盖的模块
- 搜索 for 循环内的 make(), append(), new() 调用
- 搜索 go func() 启动点，检查是否有 done channel 或 context 退出
- 检查 DB 连接池配置（gorm.Config 或 sql.DB 设置）
- 搜索 sync.Pool 使用
- 检查 HTTP client 的 Transport 配置
- 搜索 context.WithTimeout, context.WithDeadline 的使用分布
- 检查 Prometheus label 的值域是否有限
```

---

## 评分标准

每个维度评分 1-10，基于检查项通过率：

| 分数 | 含义 | 通过率 |
|------|------|--------|
| 9-10 | 生产就绪，最佳实践 | ≥90% 检查项 ✅ |
| 7-8 | 基本就绪，有小改进空间 | ≥70% 检查项 ✅ |
| 5-6 | 需要修复才能上线 | ≥50% 检查项 ✅ |
| 3-4 | 严重不足，需要大量工作 | ≥30% 检查项 ✅ |
| 1-2 | 几乎未实现 | <30% 检查项 ✅ |

状态判定：✅ 评分 ≥ 7 | ⚠️ 评分 5-6 | ❌ 评分 < 5

问题优先级判定：
- **P0**：会导致生产事故（数据丢失、安全漏洞、服务不可用）
- **P1**：影响可维护性或一致性（类型不匹配、文档过时、测试缺失）
- **P2**：改进建议（代码组织、性能优化、最佳实践）

---

## 汇总报告格式

所有维度完成后，生成以下汇总：

```markdown
## 生产就绪度审计报告

### 总评分：X.X / 10

| 维度 | 评分 | 状态 |
|------|------|------|
| 1. 生产基础设施 | X/10 | ✅/⚠️/❌ |
| 2. API 一致性 | X/10 | ✅/⚠️/❌ |
| 3. 架构分层 | X/10 | ✅/⚠️/❌ |
| 4. 接口契约 | X/10 | ✅/⚠️/❌ |
| 5. 测试质量 | X/10 | ✅/⚠️/❌ |
| 6. 入口一致性 | X/10 | ✅/⚠️/❌ |
| 7. 安全审计 | X/10 | ✅/⚠️/❌ |
| 8. 性能基线 | X/10 | ✅/⚠️/❌ |

### P0 问题（必须修复）
- [文件:行号] 问题描述 → 修复方案

### P1 问题（应该修复）
- [文件:行号] 问题描述 → 修复方案

### P2 问题（建议改进）
- [文件:行号] 问题描述 → 修复方案
```

---

## 增量审计模式

如果项目之前已经做过审计（检查 `.trellis/workspace/` 中的历史记录），可以进行增量审计：

1. 读取上次审计的问题清单
2. 仅检查上次标记为 P0/P1 的问题是否已修复
3. 对新增/修改的文件（`git diff --name-only <last-audit-commit>..HEAD`）进行全量审计
4. 输出增量报告：已修复 / 未修复 / 新发现

---

## 反模式（严格避免）

- 问用户"要审计哪些维度？"— 从参数或默认值推导
- 问用户"项目路径在哪？"— 使用当前工作目录
- 给出没有文件引用的模糊评价 — 每个发现必须有 `file:line`
- 评分基于"感觉"而非检查项通过率
- 修复建议是"建议改进"而非具体的代码修改方案
- 串行启动 agent — 必须在同一条消息中并行发送
- 审计已知问题而不先读取项目规范 — 先读 `.trellis/spec/`

---

## 与其他命令的集成

| 命令 | 关系 |
|------|------|
| `/trellis:start` | 审计完成后，可用 start 启动修复任务 |
| `/trellis:parallel` | 审计发现 P0 问题后，可用 parallel 并行修复 |
| `/trellis:check-backend` | 审计是全局视角，check-backend 是单次修改的局部检查 |
| `/trellis:check-cross-layer` | 审计包含跨层检查，但更广更深 |
| `/trellis:update-spec` | 审计发现新模式后，可用 update-spec 沉淀到规范 |
| `/trellis:finish-work` | 修复完成后，用 finish-work 做提交前检查 |

---
name: bug-analysis
description: 统一性 Bug 溯源与架构审计。当需要分析项目 Bug、排查代码一致性问题、审计架构统一性时自动触发。覆盖九大维度：错误捕获统一性、输入准入统一性、状态机统一性、观测链路统一性、前后端契约一致性、性能与资源泄漏、安全漏洞统一性、并发与事务一致性、代码分层一致性。分析结果输出到 .bug/ 目录下的 Markdown 文档。
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Task
  - Edit
  - Write
---

# 🔍 统一性 Bug 溯源与架构审计 (Universal Architecture Consistency Audit)

## 技能定位

你是一个资深架构审计师。当系统出现任何非预期行为时，不采取"头痛医头"的修复方式，而是将该 Bug 映射到系统的"统一性（Unified）"缺失上。通过九个统一维度，将单个 Bug 的修复转化为系统级健壮性的提升。

**核心理念：** 每个 Bug 都是系统"统一性裂缝"的信号。修一个 Bug 是战术，堵一类 Bug 是战略。

**🌍 多语言适配：** 本技能适用于所有主流技术栈。每个 Agent 在分析前必须先自动识别项目使用的语言和框架（通过扫描 package.json / pom.xml / build.gradle / requirements.txt / go.mod / Cargo.toml / composer.json / Gemfile 等），然后按对应技术栈的排查清单执行分析。

---

## 📁 输出规范（重要！）

**所有分析结果必须输出到 `.bug/` 目录下的 Markdown 文档。**

### 文件命名规则

```
.bug/BUG-{YYYYMMDD}-{HHmmss}-{简短描述}.md
```

示例：
- `.bug/BUG-20260301-143022-全局异常处理缺陷.md`
- `.bug/BUG-20260301-150000-输入校验缺失审计.md`

### 文档模板

每个 bug 文档必须遵循以下结构：

```markdown
# 🔍 Bug 审计报告：{标题}

> 📅 创建时间：{YYYY-MM-DD HH:mm:ss}
> 🔖 审计维度：{涉及的维度}
> 📊 总体评分：{X}/10

---

## 📊 总览仪表盘

| 维度 | 评分 | P0 | P1 | P2 |
|------|------|----|----|-----|
| 🚨 错误捕获 | ?/10 | ? 个 | ? 个 | ? 个 |
| 🛡️ 输入准入 | ?/10 | ? 个 | ? 个 | ? 个 |
| 🧩 状态机 | ?/10 | ? 个 | ? 个 | ? 个 |
| 🛰️ 观测链路 | ?/10 | ? 个 | ? 个 | ? 个 |
| 🔗 前后端契约 | ?/10 | ? 个 | ? 个 | ? 个 |
| ⚡ 性能资源 | ?/10 | ? 个 | ? 个 | ? 个 |
| 🔒 安全漏洞 | ?/10 | ? 个 | ? 个 | ? 个 |
| 🔄 并发事务 | ?/10 | ? 个 | ? 个 | ? 个 |
| 📐 代码分层 | ?/10 | ? 个 | ? 个 | ? 个 |
| **总计** | **?/10** | **? 个** | **? 个** | **? 个** |

---

## 📋 Bug 清单

### 🚨 维度一：错误捕获统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| E-001 | [ ] | `path/to/file:42` | 描述 | P0 | 建议 |
| E-002 | [ ] | `path/to/file:88` | 描述 | P1 | 建议 |

### 🛡️ 维度二：输入准入统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| V-001 | [ ] | `path/to/file:15` | 描述 | P0 | 建议 |

### 🧩 维度三：状态机统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| S-001 | [ ] | `path/to/file:33` | 描述 | P1 | 建议 |

### 🛰️ 维度四：观测链路统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| O-001 | [ ] | `path/to/file:77` | 描述 | P2 | 建议 |

### 🔗 维度五：前后端契约一致性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| C-001 | [ ] | `path/to/file:20` | 描述 | P1 | 建议 |

### ⚡ 维度六：性能与资源泄漏

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| P-001 | [ ] | `path/to/file:55` | 描述 | P0 | 建议 |

### 🔒 维度七：安全漏洞统一性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| X-001 | [ ] | `path/to/file:30` | 描述 | P0 | 建议 |

### 🔄 维度八：并发与事务一致性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| T-001 | [ ] | `path/to/file:90` | 描述 | P1 | 建议 |

### 📐 维度九：代码分层一致性

| 编号 | 状态 | 文件:行号 | 问题描述 | 严重程度 | 修复建议 |
|------|------|-----------|----------|----------|----------|
| L-001 | [ ] | `path/to/file:12` | 描述 | P2 | 建议 |

---

## 💡 系统性改进方案

### 短期修复（直接修复具体问题）
- [ ] 修复项 1
- [ ] 修复项 2

### 中期防护（自动化检查机制）
- [ ] 防护项 1
- [ ] 防护项 2

### 长期治理（架构层面提升）
- [ ] 治理项 1
- [ ] 治理项 2

---

## 📝 修复记录

| 日期 | 编号 | 修复人/工具 | 修复说明 | 关联 commit |
|------|------|-------------|----------|-------------|
| | | | | |
```

### 状态标记说明

- `[ ]` — 未修复（待处理）
- `[-]` — 修复中（进行中）
- `[x]` — 已修复（已完成）

### 输出流程

分析完成后，必须执行以下步骤：

1. **生成文件名**：按命名规则生成带时间戳的文件名
2. **写入 .bug/ 目录**：使用 Write 工具将报告写入 `.bug/` 目录
3. **在终端输出摘要**：向用户展示总览仪表盘和 P0 问题列表
4. **提示后续操作**：告知用户可以使用 `/bug-fix` 技能来逐个修复并归档

---

## ⚡ 并行执行策略 (Parallel Execution Strategy)

**核心原则：九个维度互相独立，必须并行分析，禁止串行逐个跑。分两批并行启动。**

### 执行流程

```
┌──────────────────────────────────────────────────────────────────────┐
│                       /bug-analysis 触发                              │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Phase 1: 第一批并行探测（5 个 Task agent 同时启动）                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │
│  │ Agent 1  │ │ Agent 2  │ │ Agent 3  │ │ Agent 4  │ │ Agent 5  │  │
│  │ 🚨错误捕获│ │ 🛡️输入准入│ │ 🧩状态机  │ │ 🛰️观测链路│ │ 🔗前后端  │  │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘  │
│       │            │            │            │            │          │
│  Phase 2: 第二批并行探测（4 个 Task agent 同时启动）                    │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐               │
│  │ Agent 6  │ │ Agent 7  │ │ Agent 8  │ │ Agent 9  │               │
│  │ ⚡性能资源│ │ 🔒安全漏洞│ │ 🔄并发事务│ │ 📐代码分层│               │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘               │
│       │            │            │            │                       │
│  Phase 3: 汇总合并（主 agent 收集九路结果）                             │
│       └────────────┴────────────┴────────────┘                       │
│                        │                                             │
│                  📊 生成审计报告                                       │
│                  📁 写入 .bug/ 目录                                    │
│                  💡 交叉分析 & SOP                                     │
└──────────────────────────────────────────────────────────────────────┘
```

### 并行启动指令

**分两批启动，每批在一条消息中同时发起多个 Task 调用。**

**第一批（5 个 agent 同时启动）：**

```
Task 1 (subagent_type=Explore): "维度一：错误捕获统一性分析"
  → 搜索 GlobalExceptionHandler、catch 块、ErrorBoundary、Axios 拦截器
  → 检查 Filter 层异常是否有桥接
  → 输出问题列表（文件:行号 + 严重程度）

Task 2 (subagent_type=Explore): "维度二：输入准入统一性分析"
  → 扫描所有 Controller 的 @Valid 注解
  → 检查 DTO 校验注解完整性
  → 搜索 Service 层散落的手动校验
  → 输出问题列表（文件:行号 + 严重程度）

Task 3 (subagent_type=Explore): "维度三：状态机与常量统一性分析"
  → 扫描 Magic Number/String
  → 审计所有 Enum 定义和状态流转
  → 对比前后端常量同步
  → 输出问题列表（文件:行号 + 严重程度）

Task 4 (subagent_type=Explore): "维度四：观测与链路统一性分析"
  → 检查 Trace ID / RequestId 覆盖度
  → 搜索 System.out.println / e.printStackTrace
  → 审计关键业务日志覆盖
  → 检查敏感信息泄露
  → 输出问题列表（文件:行号 + 严重程度）

Task 5 (subagent_type=Explore): "维度五：前后端契约一致性分析"
  → 对比后端 DTO/VO 字段与前端 API 调用的字段名
  → 检查字段类型匹配（Long vs string、Date 格式等）
  → 检查分页结构一致性
  → 检查响应包装结构一致性
  → 输出问题列表（文件:行号 + 严重程度）
```

**第二批（4 个 agent 同时启动）：**

```
Task 6 (subagent_type=Explore): "维度六：性能与资源泄漏分析"
  → 搜索 N+1 查询模式（循环内的数据库调用）
  → 检查 EAGER fetch 和 Cartesian 爆炸风险
  → 搜索未关闭的资源（Connection、Stream、Reader）
  → 检查缺少分页的列表查询
  → 检查前端内存泄漏（未清理的定时器、事件监听器）
  → 输出问题列表（文件:行号 + 严重程度）

Task 7 (subagent_type=Explore): "维度七：安全漏洞统一性分析"
  → 检查越权访问（缺少权限校验的接口）
  → 搜索 SQL 拼接（SQL 注入风险）
  → 检查 XSS 防护（用户输入未转义直接输出）
  → 检查 CSRF 防护配置
  → 搜索硬编码密钥/密码/Token
  → 检查 JWT 配置（过期时间、签名算法）
  → 检查 CORS 配置
  → 输出问题列表（文件:行号 + 严重程度）

Task 8 (subagent_type=Explore): "维度八：并发与事务一致性分析"
  → 搜索缺少 @Transactional 的写操作方法
  → 检查事务传播级别是否合理
  → 搜索先查后改的非原子操作（竞态条件）
  → 检查并发资源访问是否有锁保护
  → 检查异步操作的错误处理和超时控制
  → 输出问题列表（文件:行号 + 严重程度）

Task 9 (subagent_type=Explore): "维度九：代码分层一致性分析"
  → 检查 Controller 层是否包含业务逻辑
  → 检查 Service 层是否直接操作 HTTP 对象
  → 检查 Repository 层是否包含业务判断
  → 搜索循环依赖
  → 检查命名规范一致性
  → 输出问题列表（文件:行号 + 严重程度）
```

### 汇总阶段

九路结果返回后，主 agent 负责：
1. **去重合并：** 同一问题可能被多个维度发现，合并为一条
2. **交叉分析：** 检查跨维度关联（如：缺少 @Valid → 异常未捕获 → 日志无上下文 → 可被注入攻击，四连击）
3. **生成文档：** 按照输出规范模板，写入 `.bug/` 目录
4. **SOP 映射：** 每个问题标注对应的 Fix → Categorize → Generalize → Automate 步骤

### 各 Agent 详细 Prompt

**Agent 1 — 🚨 错误捕获统一性：**
```
分析项目的错误捕获与反馈统一性。先识别项目使用的技术栈，再按对应框架排查。

【后端排查】
- Java/Spring: 检查 @RestControllerAdvice + @ExceptionHandler 覆盖度、Filter 层异常桥接
- Node.js/Express: 检查 app.use((err, req, res, next)) 错误中间件、express-async-errors
- Node.js/Koa: 检查 app.on('error') 和 ctx.throw 使用
- Node.js/NestJS: 检查 @Catch() ExceptionFilter 全局注册
- Python/Django: 检查自定义 middleware 和 handler500、DRF 的 exception_handler
- Python/FastAPI: 检查 @app.exception_handler 和自定义 HTTPException
- Python/Flask: 检查 @app.errorhandler 注册
- Go/Gin: 检查 Recovery() 中间件和自定义 error handler
- Go/Echo: 检查 HTTPErrorHandler 自定义
- Rust/Actix: 检查 ResponseError trait 实现
- PHP/Laravel: 检查 App\Exceptions\Handler 和 render() 方法
- Ruby/Rails: 检查 rescue_from 和 exceptions_app 配置

【通用排查】
1. 搜索所有 catch/except/rescue/recover 块，检查空捕获、吞异常、仅打印不记录
2. 检查错误响应格式是否全局统一（{error, status, message} 结构一致性）
3. 检查中间件/过滤器层异常是否能被全局处理器捕获

【前端排查】
- React: 检查 ErrorBoundary 覆盖范围（只捕获渲染阶段，不捕获事件/异步）
- Vue: 检查 app.config.errorHandler + onErrorCaptured
- Angular: 检查自定义 ErrorHandler 实现
- Svelte: 检查 handleError hook
- 通用: 检查 HTTP 客户端拦截器（Axios/Fetch/ky）、window.onerror、window.onunhandledrejection

输出格式：| 编号 | 文件:行号 | 问题描述 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 2 — 🛡️ 输入准入统一性：**
```
分析项目的输入校验统一性。先识别项目使用的技术栈，再按对应框架排查。

【后端排查】
- Java/Spring: 扫描 Controller 的 @Valid/@Validated 注解、DTO 的 Jakarta Bean Validation 注解
- Node.js: 检查 Joi/Zod/class-validator schema 定义与路由绑定
- Python/Django: 检查 DRF Serializer 的 validate_* 方法和 Field 约束
- Python/FastAPI: 检查 Pydantic Model 的 Field 约束和 validator 装饰器
- Python/Flask: 检查 marshmallow/WTForms schema
- Go: 检查 go-playground/validator struct tag 和 validate.Struct() 调用
- Rust: 检查 serde 反序列化约束和自定义 validator
- PHP/Laravel: 检查 FormRequest 的 rules() 方法和 Validator::make
- Ruby/Rails: 检查 ActiveModel validations 和 strong parameters

【通用排查】
1. 搜索路由/控制器层的请求体参数，检查是否都经过校验层
2. 搜索业务逻辑层中散落的手动校验（if xxx == null / if not xxx / if xxx.empty?），评估是否应提升到校验层
3. 检查分页参数是否有上限保护（防止 size=999999）
4. 检查前端表单校验规则与后端校验是否对齐

输出格式：| 编号 | 文件:行号 | 缺失的校验 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 3 — 🧩 状态机统一性：**
```
分析项目的状态管理与常量统一性。适用于所有语言和框架。

【通用排查】
1. 搜索代码中的 Magic Number（数字字面量在 if/switch/match 中）和 Magic String（字符串字面量比对）
2. 列出所有枚举/常量定义（Java Enum、Python Enum/IntEnum、TS enum/const object、Go const iota、Rust enum），检查语义一致性
3. 分析业务状态流转逻辑，检查是否有非法状态跳转（如"已取消"→"已完成"）
4. 对比前端常量定义（constants.ts/js、enum 文件）与后端枚举，检查是否同步
5. 搜索硬编码配置值（超时时间、重试次数、URL、端口号、密钥长度等）
6. 检查枚举序列化方式是否统一（name vs value vs ordinal）

输出格式：| 编号 | 文件:行号 | 硬编码/不一致内容 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 4 — 🛰️ 观测链路统一性：**
```
分析项目的日志与链路追踪统一性。先识别项目使用的技术栈，再按对应框架排查。

【后端排查】
- Java/Spring: 检查 MDC + RequestFilter、logback-spring.xml/log4j2.xml 配置
- Node.js: 检查 winston/pino/bunyan 配置、cls-hooked/AsyncLocalStorage 上下文传递
- Python: 检查 logging 模块配置、structlog、contextvars 上下文传递
- Go: 检查 zap/logrus/slog 配置、context.Context 传递
- Rust: 检查 tracing/log crate 配置
- PHP/Laravel: 检查 Log facade 和 Monolog 配置
- Ruby/Rails: 检查 Rails.logger 和 Tagged Logging

【通用排查 — 反模式搜索】
1. 搜索直接打印语句：System.out.println、console.log（生产代码中）、print()（Python 非 logging）、fmt.Println（Go 非 log）
2. 搜索 e.printStackTrace()、traceback.print_exc()、console.error(err) 等未接入日志框架的异常输出
3. 检查 Trace ID / Request ID 是否贯穿全链路（含异步任务、消息队列、WebSocket）
4. 检查关键业务操作（认证、支付、状态变更）是否有充分的结构化日志
5. 检查异常日志是否包含上下文（userId、请求参数），是否丢失堆栈
6. 检查日志中是否泄露敏感信息（密码、Token、手机号、身份证号）

输出格式：| 编号 | 文件:行号 | 日志问题 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 5 — 🔗 前后端契约一致性：**
```
分析项目的前后端 API 契约一致性。适用于所有前后端分离架构。

【后端响应结构排查】
- Java/Spring: 对比 Controller 返回的 DTO/VO 字段
- Node.js/Express/NestJS: 对比 response 对象或 DTO class 字段
- Python/Django DRF: 对比 Serializer 字段
- Python/FastAPI: 对比 Pydantic response_model 字段
- Go: 对比 struct json tag 字段
- PHP/Laravel: 对比 Resource/JsonResponse 字段
- Ruby/Rails: 对比 jbuilder/ActiveModel Serializer 字段

【前端消费排查】
- React/Vue/Angular/Svelte: 检查 API 调用层（axios/fetch/ky/HttpClient）的请求参数和响应解构
- TypeScript 项目: 检查 interface/type 定义与后端是否同步
- 移动端（React Native/Flutter/Swift/Kotlin）: 检查 Model 定义与后端是否同步

【通用排查】
1. 字段命名不一致（camelCase vs snake_case vs PascalCase、userName vs username vs user_name）
2. 字段类型不匹配（后端 int64/Long 前端当 string、日期格式不统一）
3. 分页结构不对齐（pageNum/page、total/totalCount、size/pageSize）
4. 响应包装结构不一致（code/message/data 的嵌套层级）
5. 可选/必填字段认知不一致（后端必填，前端可能传 null/undefined/nil）
6. 枚举值传递不匹配（name vs value vs ordinal）
7. 文件接口契约（Content-Type、multipart 参数名、文件大小限制）

输出格式：| 编号 | 文件:行号 | 契约不一致描述 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 6 — ⚡ 性能与资源泄漏：**
```
分析项目的性能隐患与资源泄漏。先识别项目使用的技术栈，再按对应语言排查。

【后端 ORM/数据库排查】
- Java/JPA/Hibernate: N+1 查询、EAGER FetchType 笛卡尔积、未关闭 EntityManager
- Node.js/Prisma: 嵌套 include 过深、未使用 select 精确查询
- Node.js/TypeORM: eager relations、QueryBuilder 循环调用
- Node.js/Sequelize: 循环内 findOne、include 过深
- Python/Django ORM: select_related/prefetch_related 缺失导致 N+1、QuerySet 未 lazy 评估
- Python/SQLAlchemy: lazy='select' 导致 N+1、Session 未关闭
- Go/GORM: Preload 缺失、循环内查询
- PHP/Laravel Eloquent: with() 缺失导致 N+1、chunk() 未使用
- Ruby/Rails ActiveRecord: includes/preload 缺失、each 内触发查询

【资源泄漏排查 — 通用】
1. 搜索未关闭的资源：数据库连接、文件句柄、HTTP 连接、Stream/Reader/Writer
   - Java: 未使用 try-with-resources
   - Python: 未使用 with 语句
   - Go: 未 defer Close()
   - Node.js: 未 destroy() Stream
   - Rust: 通常由 Drop trait 处理，检查 unsafe 块
2. 检查列表查询是否缺少分页（findAll/all/find({}) 无 limit）
3. 检查缓存使用：重复查询未缓存、缓存无 TTL/过期时间

【前端性能排查】
1. 未清理的 setInterval/setTimeout（组件卸载时）
2. 未移除的事件监听器（addEventListener 无对应 removeEventListener）
3. 未取消的 HTTP 请求（AbortController/CancelToken）
4. React: 未清理的 useEffect 副作用、不必要的 re-render
5. Vue: 未销毁的 watcher、未清理的 eventBus 监听
6. 大列表未虚拟化（react-virtualized/vue-virtual-scroller）

输出格式：| 编号 | 文件:行号 | 性能问题描述 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 7 — 🔒 安全漏洞统一性：**
```
分析项目的安全漏洞（参考 OWASP Top 10 2025）。先识别项目使用的技术栈，再按对应框架排查。

【越权访问 — Broken Access Control（OWASP #1）】
- Java/Spring: 检查 @PreAuthorize/@Secured/@RolesAllowed 注解、SecurityConfig 配置
- Node.js/Express: 检查 auth middleware 是否覆盖所有路由
- Python/Django: 检查 @permission_required/@login_required、DRF permission_classes
- Python/FastAPI: 检查 Depends() 依赖注入的权限校验
- Go: 检查 middleware 权限校验链
- PHP/Laravel: 检查 Gate/Policy、middleware('auth')
- Ruby/Rails: 检查 before_action :authenticate_user!、Pundit/CanCanCan
- 通用: 检查水平越权（用户A访问用户B的数据，是否校验资源归属）

【注入攻击】
- SQL 注入: 搜索字符串拼接 SQL（非参数化查询/预编译语句）
- ORM 注入: 搜索 raw SQL/原生查询中的字符串拼接
- NoSQL 注入: 检查 MongoDB 查询中的用户输入是否直接传入 $where/$regex
- 命令注入: 搜索 exec/system/spawn/os.system/subprocess 中拼接用户输入
- LDAP/XPath 注入: 搜索相关查询中的字符串拼接

【XSS 防护】
- 检查用户输入是否未经转义直接输出（innerHTML/dangerouslySetInnerHTML/v-html/{!! !!}）
- 检查 Content-Security-Policy 响应头配置

【CSRF 防护】
- Java/Spring: 检查 Spring Security CSRF 配置
- Node.js: 检查 csurf/csrf-csrf 中间件
- Python/Django: 检查 CsrfViewMiddleware 和 {% csrf_token %}
- PHP/Laravel: 检查 @csrf 和 VerifyCsrfToken middleware
- Ruby/Rails: 检查 protect_from_forgery

【通用安全排查】
1. 硬编码敏感信息: 搜索代码中的密码、API Key、JWT Secret、数据库连接字符串
2. JWT 安全: Token 过期时间、签名算法强度（禁止 none/HS256 弱密钥）、Refresh Token 机制
3. CORS 配置: allowedOrigins/Access-Control-Allow-Origin 是否为 "*"
4. 敏感数据暴露: API 响应中不必要的敏感字段（密码哈希、内部 ID、手机号明文）
5. 依赖安全: 是否有已知漏洞的依赖（检查 package.json/pom.xml/requirements.txt/go.mod）

输出格式：| 编号 | 文件:行号 | 安全问题描述 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 8 — 🔄 并发与事务一致性：**
```
分析项目的并发安全与事务管理。先识别项目使用的技术栈，再按对应框架排查。

【事务管理排查】
- Java/Spring: 检查 @Transactional 缺失、传播级别（REQUIRED/REQUIRES_NEW/NESTED）、readOnly 标记
- Node.js/Prisma: 检查 $transaction 使用、交互式事务 vs 批量事务
- Node.js/TypeORM: 检查 QueryRunner/transaction manager 使用
- Node.js/Sequelize: 检查 sequelize.transaction() 使用
- Python/Django: 检查 @transaction.atomic 装饰器、transaction.on_commit
- Python/SQLAlchemy: 检查 session.begin()、session.commit/rollback 配对
- Go/GORM: 检查 db.Transaction() 使用
- PHP/Laravel: 检查 DB::transaction() 使用
- Ruby/Rails: 检查 ActiveRecord::Base.transaction 使用

【并发安全排查 — 通用】
1. 竞态条件: 搜索"先查后改"模式（先 find/get 再 save/update），检查并发修改同一资源的风险
2. 乐观锁/悲观锁: 检查关键业务实体是否有版本控制（@Version/version 字段/SELECT FOR UPDATE/LOCK）
3. 非原子操作: 余额扣减、库存扣减、状态变更是否为原子操作（数据库层面 UPDATE SET x = x - 1）
4. 死锁风险: 多表更新顺序是否一致、是否存在交叉锁定

【异步操作排查】
- Java: @Async 异常处理、线程池拒绝策略、CompletableFuture 异常链
- Node.js: Promise.all 错误处理、未 await 的 Promise、EventEmitter 错误事件
- Python: asyncio 任务异常处理、ThreadPoolExecutor 异常捕获
- Go: goroutine panic recovery、channel 死锁、WaitGroup 使用
- Rust: tokio task 异常处理、async 取消安全

输出格式：| 编号 | 文件:行号 | 并发/事务问题 | 严重程度(P0/P1/P2) | 修复建议 |
```

**Agent 9 — 📐 代码分层一致性：**
```
分析项目的代码分层与架构规范。先识别项目使用的架构模式，再按对应模式排查。

【MVC/分层架构排查（Java Spring/PHP Laravel/Ruby Rails/Python Django/Go 等）】
1. 路由/控制器层越界: 是否包含业务逻辑（复杂 if/else、直接调用数据层、数据转换）
2. 业务/服务层越界: 是否直接操作 HTTP 请求/响应对象（Request/Response/Session/Cookie）
3. 数据/仓储层越界: 是否包含业务判断逻辑（应只负责数据存取）

【Node.js/NestJS 分层排查】
1. Controller 是否包含业务逻辑（应委托给 Service）
2. Service 是否直接操作 @Req()/@Res() 对象
3. Module 之间是否存在循环依赖

【前端架构排查】
1. React: 组件是否混合了数据获取和 UI 渲染（应分离 hooks/services）
2. Vue: 组件是否直接调用 API（应通过 store/composable）
3. Angular: Component 是否包含业务逻辑（应委托给 Service）

【通用排查】
1. 循环依赖: 模块/服务之间是否存在循环引用（A→B→A）
2. 命名规范一致性: 路由方法（get/list/create/update/delete）、服务方法、数据模型命名是否统一
3. 分包/分目录规范: 文件是否放对了目录（如 DTO 放在 model 目录、工具类放在 service 目录）
4. 依赖方向: 是否存在下层依赖上层（数据层引用业务层、模型引用 DTO）
5. 关注点分离: 日志、认证、缓存等横切关注点是否通过中间件/装饰器/AOP 统一处理

输出格式：| 编号 | 文件:行号 | 分层问题描述 | 严重程度(P0/P1/P2) | 修复建议 |
```

---

## 📐 九大统一维度

### 维度一：🚨 错误捕获与反馈统一性 (Unified Fault Tolerance)

**分析角度：** Bug 是否是因为异常"掉到了地上"？

**跨语言/框架对照表：**

| 技术栈 | 全局异常拦截机制 | 关键注意点 |
|--------|-----------------|-----------|
| Java/Spring | `@RestControllerAdvice` + `@ExceptionHandler` | Filter 层异常需桥接到 HandlerExceptionResolver |
| Node.js/Express | `app.use((err, req, res, next))` | 异步异常需 express-async-errors 或手动 next(err) |
| Node.js/NestJS | `@Catch()` ExceptionFilter | 需全局注册 useGlobalFilters |
| Python/Django | Middleware + handler500 / DRF exception_handler | 中间件顺序影响异常捕获 |
| Python/FastAPI | `@app.exception_handler` | Starlette 中间件异常需单独处理 |
| Go/Gin | `Recovery()` middleware | goroutine 内 panic 不被外层 recover 捕获 |
| PHP/Laravel | `App\Exceptions\Handler` | render() 方法需覆盖所有异常类型 |
| Ruby/Rails | `rescue_from` in ApplicationController | 中间件层异常需 exceptions_app |
| Rust/Actix | `ResponseError` trait | 需为所有自定义错误类型实现 |
| React | `ErrorBoundary` (class component) | 只捕获渲染阶段，不捕获事件/异步 |
| Vue | `app.config.errorHandler` | 需额外处理 Promise rejection |
| Angular | 自定义 `ErrorHandler` | 需注入到 root module |

**排查清单：**
1. **全局拦截覆盖度：** 全局异常处理器是否覆盖了所有异常/错误类型
2. **中间件/过滤器层盲区：** 中间件层抛出的异常是否能被全局处理器捕获
3. **吞异常检测：** 搜索所有 catch/except/rescue/recover 块，检查空捕获、仅打印不记录、捕获后返回 null/None/nil
4. **响应格式一致性：** 所有错误响应是否遵循统一结构
5. **前端兜底完整性：** ErrorBoundary/errorHandler、HTTP 拦截器、window.onunhandledrejection

### 维度二：🛡️ 输入准入统一性 (Unified Validation Layer)

**分析角度：** Bug 是否是因为"信任了不该信任的数据"？

**跨语言/框架对照表：**

| 技术栈 | 统一校验机制 | 关键注意点 |
|--------|-------------|-----------|
| Java/Spring | Jakarta Bean Validation (`@Valid` + Hibernate Validator) | `@Valid` 缺失则校验不生效 |
| Node.js | Joi / Zod / class-validator | Schema 定义要与 DTO 同步 |
| Python/Django DRF | Serializer + Field 约束 | validate_* 方法需覆盖复杂规则 |
| Python/FastAPI | Pydantic Model + Field | validator 装饰器处理复杂校验 |
| Go | go-playground/validator | 需手动调用 validate.Struct() |
| PHP/Laravel | FormRequest rules() | 自定义 Rule 处理复杂校验 |
| Ruby/Rails | ActiveModel validations | strong parameters 防止批量赋值 |
| Rust | serde + validator crate | 编译期类型安全 + 运行时校验 |
| React | Ant Design Form / react-hook-form + Zod | 前端校验不能替代后端 |
| Vue | Element Plus rules / VeeValidate + Zod | 同上 |

**排查清单：**
1. **校验注解/Schema 完整性：** 路由层请求参数是否都经过校验
2. **校验规则充分性：** 校验规则是否覆盖业务需求（长度、范围、格式、自定义规则）
3. **校验逻辑散落检测：** 业务层中的手动校验是否应提升到校验层
4. **分页/查询参数安全：** 分页 size 上限、排序字段白名单、搜索关键词长度限制
5. **前后端校验对齐：** 前端表单校验规则与后端校验是否一致

### 维度三：🧩 状态机与常量统一性 (Unified State Machine)

**分析角度：** Bug 是否是因为"对 1 的理解不同"？

**跨语言枚举/常量对照表：**

| 语言 | 枚举/常量机制 | 序列化注意点 |
|------|-------------|-------------|
| Java | `enum` | name() vs ordinal() vs 自定义值 |
| TypeScript | `enum` / `const object` / `as const` | 编译后可能丢失类型信息 |
| Python | `Enum` / `IntEnum` / `StrEnum` | .value vs .name 序列化差异 |
| Go | `const` + `iota` | 无原生 enum，需自定义类型 |
| Rust | `enum` | serde 序列化策略需显式配置 |
| PHP | `enum` (8.1+) / `const` | backed enum vs unit enum |
| Ruby | 无原生 enum，常用 `freeze` 常量或 gem | 字符串比对易出错 |
| C# | `enum` | 默认 int 序列化，需 [JsonConverter] |

**排查清单：**
1. **Magic Number/String 扫描：** 数字/字符串字面量在条件判断中直接使用
2. **枚举/常量一致性审计：** 同一概念在不同模块是否定义一致
3. **状态流转合法性：** 是否有非法状态跳转、是否有状态机定义
4. **前后端常量同步：** 前端常量定义与后端枚举是否对齐
5. **配置外部化：** 硬编码的超时、重试、URL 等是否应提取到配置文件/环境变量

### 维度四：🛰️ 观测与链路统一性 (Unified Observability)

**分析角度：** 排障时是否在"盲人摸象"？

**跨语言日志/追踪对照表：**

| 技术栈 | 日志框架 | 上下文传递机制 |
|--------|---------|--------------|
| Java/Spring | SLF4J + Logback/Log4j2 | MDC + RequestFilter |
| Node.js | winston / pino / bunyan | cls-hooked / AsyncLocalStorage |
| Python | logging / structlog | contextvars |
| Go | zap / logrus / slog | context.Context |
| Rust | tracing / log | tracing::Span |
| PHP/Laravel | Monolog (Log facade) | Context::add() |
| Ruby/Rails | Rails.logger | Tagged Logging |

**反模式搜索关键词：**
- Java: `System.out.println`, `e.printStackTrace()`
- Node.js: `console.log` (生产代码), `console.error(err)` (未接入日志框架)
- Python: `print()` (非 logging), `traceback.print_exc()`
- Go: `fmt.Println` / `fmt.Printf` (非 log/slog)
- PHP: `echo`, `var_dump`, `print_r`
- Ruby: `puts`, `p`, `pp`

**排查清单：**
1. **Trace ID 覆盖度：** 每个请求是否有唯一 ID 贯穿全链路
2. **日志输出规范：** 是否统一使用日志框架，无直接打印语句
3. **关键业务日志覆盖：** 认证、支付、状态变更等关键操作是否有结构化日志
4. **异常日志上下文：** 异常日志是否包含 userId、请求参数、完整堆栈
5. **敏感信息泄露：** 日志中是否明文打印密码、Token、手机号

### 维度五：🔗 前后端契约一致性 (API Contract Consistency)

**分析角度：** Bug 是否是因为前后端"各说各话"？

**排查清单：**
1. **字段命名一致性：** 后端响应字段名与前端 API 调用字段名是否匹配（camelCase vs snake_case、大小写差异）
2. **字段类型匹配：** 数值类型精度（int64/Long vs JS number 精度丢失）、Date 格式（timestamp/ISO 8601/yyyy-MM-dd）、Boolean vs int
3. **分页结构对齐：** pageNum/page、total/totalCount、size/pageSize 等字段名是否前后端一致
4. **响应包装结构：** 统一响应体的 code/message/data 结构与前端解包逻辑是否对齐
5. **可选/必填认知：** 后端必填字段，前端是否可能传 null/undefined/nil/None
6. **枚举值传递：** 枚举序列化方式（name/value/ordinal）与前端使用是否匹配
7. **文件接口契约：** 上传/下载的 Content-Type、参数格式、文件大小限制是否前后端一致

**统一性自查核心问题：**
> 后端改了一个响应字段名，前端会不会静默失败？有没有类型安全的契约保障？

### 维度六：⚡ 性能与资源泄漏 (Performance & Resource Leak)

**分析角度：** 代码在开发环境跑得好好的，上了生产就炸？

**排查清单：**
1. **N+1 查询：** 循环内调用数据层方法、ORM 懒加载在迭代中触发（各 ORM 均适用）
2. **贪婪加载爆炸：** ORM 关联的 eager/include 过深导致级联查询或笛卡尔积
3. **资源泄漏：** 数据库连接、文件句柄、HTTP 连接、Stream 等未正确关闭（Java try-with-resources / Python with / Go defer / Node.js destroy）
4. **无分页全表扫描：** findAll/all/find({}) 无 limit，数据量大时 OOM
5. **缓存缺失/泄漏：** 重复查询未缓存、缓存无 TTL/过期时间
6. **前端内存泄漏：** 未清理的定时器、未移除的事件监听器、组件卸载时未取消请求、大列表未虚拟化
7. **大数据量处理：** 批量操作未分批、大文件未流式处理

**统一性自查核心问题：**
> 这个查询在 10 条数据时没问题，10 万条时还能跑吗？资源用完有没有还回去？

### 维度七：🔒 安全漏洞统一性 (Security Vulnerability — OWASP Top 10)

**分析角度：** 系统是否在"裸奔"？

**排查清单：**
1. **越权访问（Broken Access Control）：** 接口是否有权限校验（注解/中间件/装饰器）、是否存在水平越权
2. **注入攻击：** SQL/ORM 原生查询拼接、NoSQL 注入、命令注入（exec/system/subprocess）
3. **XSS 防护：** 用户输入未转义直接输出（innerHTML/v-html/dangerouslySetInnerHTML/{!! !!}）、CSP 配置
4. **CSRF 防护：** 框架 CSRF 中间件配置、非 GET 接口的 Token 保护
5. **硬编码敏感信息：** 代码中的密码、API Key、JWT Secret、数据库连接字符串
6. **JWT 安全：** Token 过期时间、签名算法强度、Refresh Token 机制
7. **CORS 配置：** Access-Control-Allow-Origin 是否为 "*"、credentials 组合安全性
8. **敏感数据暴露：** API 响应中不必要的敏感字段（密码哈希、手机号明文）
9. **依赖安全：** 是否有已知漏洞的依赖包

**统一性自查核心问题：**
> 一个普通用户能不能通过改 URL 参数访问到管理员的数据？密钥是不是写死在代码里？

### 维度八：🔄 并发与事务一致性 (Concurrency & Transaction)

**分析角度：** 两个人同时操作，数据会不会乱？

**排查清单：**
1. **事务保护缺失：** 多个写操作的业务方法是否有事务保护（@Transactional / transaction() / atomic / db.Transaction）
2. **事务传播/嵌套：** 嵌套事务的行为是否合理
3. **竞态条件：** "先查后改"模式是否有并发风险（如两人同时接单）
4. **乐观锁/悲观锁：** 关键业务实体是否有版本控制或行锁（version 字段 / SELECT FOR UPDATE）
5. **非原子操作：** 余额扣减、库存扣减、状态变更是否为数据库层面的原子操作
6. **异步操作安全：** 异步任务的异常处理、超时控制、取消机制（各语言异步模型均适用）
7. **死锁风险：** 多表更新顺序是否一致

**统一性自查核心问题：**
> 两个用户同时抢同一个任务，会不会都抢到？余额扣减会不会扣成负数？

### 维度九：📐 代码分层一致性 (Architecture Layer Consistency)

**分析角度：** 代码是不是"乱炖"？

**排查清单：**
1. **路由/控制器层越界：** 包含业务逻辑、直接调用数据层、数据转换
2. **业务/服务层越界：** 直接操作 HTTP 请求/响应对象（Request/Response/Session/Cookie）
3. **数据/仓储层越界：** 包含业务判断逻辑
4. **循环依赖：** 模块/服务之间循环引用（A→B→A）
5. **命名规范：** 路由方法/服务方法/数据模型命名是否统一
6. **分包/分目录规范：** 文件是否放对了目录
7. **依赖方向：** 是否存在下层依赖上层（数据层引用业务层）

**统一性自查核心问题：**
> 新人来了能不能一眼看懂代码结构？每一层的职责是否清晰？

---

## 📈 Bug 修复 SOP（Fix → Categorize → Generalize → Automate → Document）

- **Step 1 🔧 修复：** 修正当前 Bug，编写测试，确认不引入新问题
- **Step 2 🏷️ 归类：** 将 Bug 归入九个统一性维度之一
- **Step 3 🔎 泛化：** 搜索代码库中同类"非统一"写法
- **Step 4 🤖 自动化：** 建立对应维度的自动化防护
- **Step 5 📝 文档：** 更新工程规范，添加 CI 检查

---

## 🎯 修复优先级定义

- **P0（立即修复）：** 会导致崩溃、数据丢失、安全漏洞、资金错误
- **P1（尽快修复）：** 影响用户体验、数据一致性、排障效率
- **P2（建议改进）：** 代码质量、可维护性、规范一致性

---

## ⚠️ 分析纪律

- **必须阅读实际代码**，不要凭猜测下结论
- **每个问题必须给出精确的文件路径和行号**
- **优先关注会导致运行时错误的问题**
- 分析完成后必须将报告写入 `.bug/` 目录
- 向用户展示摘要并提示使用 `/bug-fix` 进行修复

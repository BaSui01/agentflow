# 生产就绪度多智能体深度并行审计

自适应并行审计系统：AI 根据项目特征自动判断需要 4~20 个并行审计智能体，对项目进行全方位、多层次的生产就绪度深度评估。

* **行动先于提问**（先审计，再汇总，不问元问题）
* **智能并行**（AI 分析项目后自动决定并行数量，最多 20 个，不浪费资源）
* **深度优于广度**（每个维度深入到实现细节，不做表面检查）
* **发现驱动修复**（审计结果直接产出 P0/P1/P2 修复清单，附可执行代码）
* **交叉验证**（多个维度的发现互相印证，减少误报）
* **可复用可增量**（支持按维度选择，支持增量审计，支持历史对比）

---

## 参数

- `$ARGUMENTS` — 可选，指定审计范围。示例：
  - 空（默认）：AI 自动分析项目并决定启动哪些维度（4~20 个）
  - `infra,api`：仅审计指定维度
  - `full`：强制启动全部 20 个维度
  - `all-fix`：全量审计 + 自动修复 P0 问题
  - `delta`：增量审计（仅审计上次以来的变更）
  - 可用维度标签：`infra`, `api`, `arch`, `contract`, `test`, `entry`, `security`, `perf`, `concurrency`, `error`, `observability`, `config`, `deps`, `docs`, `migration`, `llm`, `agent`, `rag`, `deploy`, `resilience`

---

## 核心原则（不可妥协）

1. **行动先于提问**
   不要问"要审计哪些维度？"或"项目路径在哪？"。从工作目录和 git 状态推导一切。

2. **智能并行，不盲目并行**
   根据项目分析结果决定启动多少个 agent（4~20）。小项目不需要 20 个 agent，大项目不应只有 8 个。

3. **具体到文件和行号**
   每个发现必须引用具体文件路径和行号。"可能有问题"不是有效发现。

4. **评分基于事实**
   评分必须基于检查项的通过/失败数量，不是主观印象。

5. **修复建议可执行**
   每个问题必须附带具体的修复方案（改哪个文件、怎么改、改成什么），不是泛泛建议。

6. **不重复已知信息**
   如果项目有 `.trellis/spec/` 规范，先读取，避免审计已记录的已知问题。

7. **交叉验证**
   汇总时对比不同维度的发现，标记矛盾或互相印证的结论。

---

## 工具策略（审计 Agent 必读）

审计 agent 拥有以下工具，按优先级使用：

### 代码理解工具（优先使用）

| 工具 | 用途 | 优先级 |
|------|------|--------|
| `mcp__ace-tool__search_context` | 语义代码搜索，理解代码意图和关系 | ⭐⭐⭐ 首选 |
| `Grep` | 精确字符串/正则匹配 | ⭐⭐ 精确搜索 |
| `Glob` | 按文件名模式查找 | ⭐⭐ 文件定位 |
| `Read` | 读取文件内容 | ⭐⭐ 深入分析 |

### 外部知识工具（按需使用）

| 工具 | 用途 | 场景 |
|------|------|------|
| `mcp__exa__web_search_exa` | 搜索最新安全漏洞、最佳实践 | 安全审计、依赖审计 |
| `mcp__exa__get_code_context_exa` | 搜索外部代码示例和文档 | 对比最佳实践 |
| `mcp__context7__resolve-library-id` + `mcp__context7__query-docs` | 查询库的最新 API 文档 | 验证 API 使用正确性 |

### 工具使用原则

1. **语义优先**：理解代码意图时，先用 `mcp__ace-tool__search_context`，再用 Grep/Glob 补充精确匹配
2. **外部验证**：安全审计和依赖审计时，用 `mcp__exa__web_search_exa` 搜索最新 CVE 和安全公告
3. **文档校验**：检查库 API 使用是否正确时，用 `mcp__context7__query-docs` 获取最新文档
4. **Bash 执行**：需要运行 `go build`, `go vet`, `golangci-lint` 等工具时使用 Bash
5. **并行搜索**：多个独立搜索应在同一条消息中并行发送

### 审计 Agent 工具配置

```
# 审计 agent 使用 research 类型（只读，不修改代码）
Task(
  subagent_type: "research",
  model: "sonnet",
  tools: [Read, Glob, Grep, Bash(只读命令),
          mcp__ace-tool__search_context,
          mcp__exa__web_search_exa,
          mcp__exa__get_code_context_exa,
          mcp__context7__resolve-library-id,
          mcp__context7__query-docs]
)
```

---

## 执行流程

### 步骤 0：自动上下文收集与项目画像

在启动任何 agent 之前，自己收集上下文并构建项目画像：

```bash
# 获取项目状态
python3 ./.trellis/scripts/get_context.py 2>/dev/null || true

# 读取项目规范（如果存在）
cat .trellis/spec/backend/index.md 2>/dev/null || true
cat .trellis/spec/backend/quality-guidelines.md 2>/dev/null || true
cat .trellis/spec/backend/error-handling.md 2>/dev/null || true
```

同时用 Glob/Grep/ace-tool 快速扫描：

```
# 项目画像扫描（并行执行）

## 结构扫描（Glob）
- Glob("**/*.go") → 统计 Go 文件数量和分布
- Glob("**/Dockerfile*") → 是否有容器化
- Glob("**/*_test.go") → 测试文件分布
- Glob("**/helm/**") or Glob("**/k8s/**") → 是否有 K8s 部署
- Glob("**/openapi*") or Glob("**/swagger*") → 是否有 API spec
- Glob("**/*.proto") → 是否有 gRPC
- Glob("**/rag/**") → 是否有 RAG 模块
- Glob("**/*.sql") or Glob("**/migration*") → 数据库迁移
- Glob("**/.github/workflows/*") or Glob("**/.gitlab-ci*") → CI/CD 配置
- Glob("**/go.mod") → 依赖数量

## 密度扫描（Grep）
- Grep("go func(") → goroutine 使用密度
- Grep("interface {") → 接口定义数量
- Grep("llm|LLM|provider") → LLM 集成密度
- Grep("agent|Agent") → Agent 模块密度
- Grep("circuit.?breaker|retry|backoff") → 韧性模式密度
- Grep("prometheus|opentelemetry|otel") → 可观测性密度

## 语义扫描（ace-tool，理解架构意图）
- mcp__ace-tool__search_context("项目的核心架构和入口点") → 理解整体架构
- mcp__ace-tool__search_context("错误处理模式和错误类型定义") → 理解错误处理策略
- mcp__ace-tool__search_context("安全相关的中间件和认证机制") → 理解安全架构

## 静态分析（Bash，可选）
- go build ./... 2>&1 | head -50 → 编译错误
- go vet ./... 2>&1 | head -50 → 静态分析问题
- golangci-lint run ./... --max-same-issues 3 2>&1 | tail -50 → lint 问题概览
```

从上下文中提取项目画像：
- 项目语言、框架、规模（文件数、包数）
- 已有的质量规范和已知问题
- 最近的提交历史（判断活跃模块）
- 模块分布特征（哪些模块最大、最复杂）
- 基础设施特征（容器化、K8s、CI/CD）
- 领域特征（是否有 LLM/Agent/RAG 等特定领域模块）

### 步骤 1：AI 智能决策审计维度

根据项目画像，从 20 个可用维度中选择最相关的维度。决策逻辑：

```
决策矩阵（按项目特征自动选择）：

基础维度（任何项目都审计，4 个）：
  ✅ arch        — 有 Go 代码就审计
  ✅ security    — 有 Go 代码就审计
  ✅ error       — 有 Go 代码就审计
  ✅ test        — 有 Go 代码就审计

条件维度（按项目特征启用，0~16 个）：
  IF 有 HTTP handler/路由注册     → api, entry
  IF 有 Dockerfile/K8s/Helm       → infra, deploy
  IF 有 interface 定义 > 5 个     → contract
  IF 有 go func() > 10 处         → concurrency
  IF 有 OpenTelemetry/Prometheus  → observability
  IF 有 config 加载逻辑           → config
  IF go.mod 依赖 > 30 个          → deps
  IF 有 migration/SQL 文件        → migration
  IF 有 LLM provider 集成         → llm
  IF 有 Agent 模块                → agent
  IF 有 RAG 模块                  → rag
  IF 有 Benchmark 或性能敏感代码  → perf
  IF 有 circuit breaker/retry     → resilience
  IF 有 README/docs 目录          → docs

最终并行数 = min(选中维度数, 20)
```

输出决策日志（告知用户）：
```
项目画像分析完成：
- Go 文件: XXX 个，分布在 XX 个包
- 检测到特征: [HTTP API, K8s 部署, LLM 集成, Agent 框架, RAG 系统, ...]
- 决定启动 N 个并行审计维度: [dim1, dim2, ...]
- 跳过维度: [dimX (原因), dimY (原因), ...]
```

### 步骤 2：创建 Team 并启动并行审计

使用 Agent Team 模式：

1. `TeamCreate` — 创建审计团队
2. `TaskCreate` × N — 为每个选中维度创建任务
3. `Task` × N — **在同一条消息中**并行启动所有 research agent

```
Task(
  subagent_type: "research",
  model: "sonnet",
  name: "audit-{dimension-label}",
  team_name: "{team-name}",
  run_in_background: true,
  prompt: "{维度 prompt}\n\n项目根目录: {cwd}\n已知规范: {spec-summary}\n\n请开始审计。"
)
```

> 关键：所有 Task 调用必须在同一条消息中发送，实现真正并行。最多 20 个并行。

### 步骤 3：等待结果并交叉验证汇总

- 每个 agent 完成后通过 SendMessage 发送报告
- 收齐所有报告后，进行交叉验证：
  - 安全审计发现的问题是否在错误处理审计中也被标记？
  - 并发审计发现的 goroutine 泄漏是否在性能审计中也被发现？
  - API 审计发现的不一致是否在契约审计中也被标记？
- 合并重复发现，标记互相印证的高置信度问题
- 汇总为总报告
- Shutdown 所有 agent → TeamDelete

### 步骤 4：分层修复建议（或自动修复）

汇总报告后：
- 按影响范围分层：全局问题 > 模块级问题 > 文件级问题
- 按修复依赖排序：先修基础问题（如错误类型统一），再修上层问题
- 列出 P0/P1/P2 问题清单，每个问题附带：
  - 具体修复方案（改哪个文件、哪一行、改成什么）
  - 修复依赖关系（先修 A 才能修 B）
  - 预估影响范围（改动会影响哪些其他文件）
- 如果 `$ARGUMENTS` 含 `all-fix`，为每个 P0 问题启动 implement agent 自动修复

---

## 审计维度定义（20 个维度）

每个审计 agent 收到的 prompt 必须包含：
1. 角色定义和维度名称
2. 具体检查项列表（带审计方法）
3. 输出格式要求
4. 项目根目录路径
5. 已知规范摘要（避免重复审计）

### 通用输出格式（所有维度共用）

```
对每个检查项给出：
- 状态：✅ 已实现 | ⚠️ 部分实现 | ❌ 缺失
- 文件引用：具体文件路径:行号
- 发现的问题（如有）：描述 + 严重程度（P0/P1/P2）
- 修复建议（如有）：具体改哪个文件、怎么改、改成什么代码

最后给出：
- 维度总评分（1-10）
- 问题清单（按 P0 > P1 > P2 排序）
- 与其他维度的交叉关联（如："此问题可能也影响安全维度"）

完成后用 SendMessage 将完整报告发送给 team-lead。
用 TaskUpdate 将任务标记为 completed。
```

---

### 维度 1：生产基础设施 (`infra`)

```
你是生产基础设施审计专家。

## 检查项
1. 健康检查端点（liveness /healthz, readiness /readyz, startup 三种探针）
2. Prometheus metrics 覆盖率（HTTP/LLM/Agent/Cache/DB 五大维度）
3. 数据库迁移机制（up/down 配对、回滚能力、dirty 状态修复）
4. 配置热重载（并发安全性、TOCTOU 防护、回滚机制）
5. TLS 配置（最低版本 TLS 1.2+、密码套件、证书轮换）
6. Docker 构建（多阶段构建、非 root 用户、HEALTHCHECK、最小基础镜像）
7. 限流机制（全局 + 租户级限流、算法选择）
8. 熔断机制（三态熔断器 Closed/Open/HalfOpen、降级策略）
9. 资源限制（内存/CPU limits、OOM 防护、ulimit 配置）
10. 日志轮转和归档策略
11. Graceful shutdown（连接排空、超时控制、信号处理）
12. 服务发现与注册（如适用）

## 审计方法
- ace-tool: search_context("健康检查端点实现和探针配置") → 理解健康检查架构
- ace-tool: search_context("限流和熔断机制的实现") → 理解韧性架构
- Grep: /health, /healthz, /ready, /readyz → 精确定位端点
- Grep: prometheus.NewCounter, prometheus.NewHistogram → metrics 注册
- Grep: migration, migrate → 迁移代码
- Grep: tls.Config, MinVersion → TLS 配置
- Glob: **/Dockerfile* → Docker 构建文件
- Read: Dockerfile → 逐层检查构建阶段
- Bash: go build ./... 2>&1 | head -20 → 编译验证
```

### 维度 2：API 一致性 (`api`)

```
你是 API 一致性审计专家。

## 检查项
1. OpenAPI spec 与实际 handler 实现是否一致（端点数量、路径、方法）
2. 请求/响应类型定义与实际序列化是否匹配（字段名、类型、required、json tag）
3. 错误处理是否统一使用 Response 信封格式（所有层，含中间件）
4. 中间件链完整性（认证、限流、CORS、日志、metrics、tracing、recovery）
5. 路由注册与 OpenAPI paths 是否一一对应
6. HTTP 方法约束（GET 无 body、POST/PUT 有 Content-Type 验证）
7. 分页/排序/过滤参数是否有统一规范
8. API 版本管理策略（URL path vs Header vs Query）
9. 请求体大小限制是否配置
10. 响应压缩（gzip/brotli）是否启用

## 审计方法
- ace-tool: search_context("API 路由注册和中间件链") → 理解 API 架构
- ace-tool: search_context("错误响应格式和信封模式") → 理解错误处理一致性
- Glob: **/openapi* or **/swagger* → 定位 API spec 文件
- Grep: HandleFunc, Handle, Route → 路由注册
- Grep: json:".*" → json tag 定义
- Read: OpenAPI spec → 逐一对比路由
- context7: 如使用 chi/gin/echo 等框架，查询最新 API 文档验证用法
```

### 维度 3：架构分层 (`arch`)

```
你是架构分层审计专家。

## 检查项
1. 包依赖方向（上层依赖下层，不反向；types 不依赖业务包）
2. 接口边界清晰度（每层通过接口通信，不直接依赖实现）
3. 循环依赖检测
4. 分层违规（handler 直接访问 DB、cmd 包含业务逻辑等）
5. types 包纯净性（仅标准库依赖，无项目内 import）
6. internal 包使用合理性
7. 适配器/桥接模式使用正确性
8. 包的职责单一性（是否有 God Package）
9. 公开 API 表面积（导出符号是否过多）
10. 包命名规范（是否有 util/common/misc 等模糊命名）

## 审计方法
- 读取每个包的 import 语句，构建依赖图
- 检测循环依赖和反向依赖
- 统计每个包的导出符号数量
- 检查 cmd/ 包是否只做组装
- 分析包大小分布，标记异常大的包
```

### 维度 4：接口契约 (`contract`)

```
你是接口契约审计专家。

## 检查项
1. 核心接口（Agent, Provider, Executor, Tokenizer 等）实现完整性
2. 接口各实现是否方法签名一致
3. Provider 接口各实现是否完整（抽查 3+ 个 provider）
4. 错误类型是否统一（不同包的 Error 类型关系）
5. 核心类型跨包一致性（是否有重复定义、字段丢失）
6. 类型转换函数是否完整（所有字段都被复制）
7. context.Context 使用是否一致（是否所有接口方法第一个参数都是 ctx）
8. 接口是否遵循 Go 惯例（小接口、组合优于继承）
9. 接口的 mock/stub 是否可用于测试
10. 废弃接口是否有 Deprecated 标记和迁移路径

## 审计方法
- 搜索所有 interface 定义，列出方法签名
- 对每个接口搜索实现者，检查方法是否完整
- 对比不同包中同名类型的字段定义
- 检查类型转换函数是否遗漏字段
```

### 维度 5：测试质量 (`test`)

```
你是测试质量审计专家。

## 检查项
1. 核心包单元测试覆盖（agent, llm, workflow, rag, config, types）
2. handler 层测试（HTTP 请求/响应完整链路）
3. 集成测试（跨包交互）
4. 端到端测试覆盖
5. 基准测试覆盖（关键路径是否有 Benchmark）
6. 竞态测试（-race flag 使用）
7. 表驱动测试模式使用率
8. 测试辅助函数质量（testutil/helper 是否复用）
9. CI/CD 配置（测试是否在 CI 中运行、是否有包被排除）
10. 代码质量工具（golangci-lint 配置、govulncheck）
11. 覆盖率阈值是否合理
12. 测试数据管理（fixture/factory 模式、测试隔离性）

## 审计方法
- 搜索 *_test.go 文件分布，统计每个包的测试文件数
- 搜索 Benchmark* 函数
- 检查 CI 配置中的测试命令和排除逻辑
- 检查 .golangci.yml 配置
- 统计表驱动测试 vs 单一断言测试的比例
```

### 维度 6：入口一致性 (`entry`)

```
你是入口一致性审计专家。

## 检查项
1. CLI 入口完整性（命令注册、子命令、help 文本）
2. 根包导出一致性（公开 API 是否有二义性）
3. 配置加载流程（默认值 → 文件 → 环境变量 → 命令行 优先级）
4. 启动顺序（依赖初始化顺序是否正确）
5. 关闭顺序（资源释放顺序：服务器 → 遥测 → 数据库）
6. 版本信息注入（ldflags -X 变量与接收变量是否匹配）
7. 信号处理（SIGTERM/SIGINT graceful shutdown + 超时机制）
8. 多入口一致性（如有多个 cmd，共享逻辑是否抽取）
9. 环境变量命名规范（前缀一致性、文档完整性）
10. 启动日志（是否打印版本、配置摘要、监听地址）

## 审计方法
- 读取 cmd/ 目录下的入口文件
- 检查配置加载代码的优先级链
- 读取 Shutdown() 方法，验证关闭顺序
- 检查 Makefile 中的 ldflags
- 搜索 signal.Notify 调用
```

### 维度 7：安全审计 (`security`)

```
你是安全审计专家。参考 OWASP Top 10 2025 和 OWASP LLM Top 10 标准。

## 检查项（OWASP Top 10 + LLM 安全）
1. CORS 配置安全性（是否默认允许 *）
2. API Key 传输方式（仅 Header，不接受 Query String）
3. JWT 配置安全性（算法白名单、密钥强度、过期时间）
4. 输入验证完整性（路径参数、查询参数、请求体）
5. SQL 注入防护（参数化查询、无 raw SQL 拼接）
6. 敏感信息泄露（日志中不打印 API Key/密码/token）
7. 安全响应头（X-Frame-Options, X-Content-Type-Options, CSP, HSTS）
8. 依赖漏洞（已知 CVE）
9. 路径遍历防护
10. Rate limiting 覆盖认证端点
11. 密钥管理（硬编码检测、环境变量 vs vault）
12. SSRF 防护（外部 URL 请求是否有白名单/黑名单）
13. 反序列化安全（JSON 解码大小限制、嵌套深度限制）
14. 审计日志（关键操作是否有不可篡改的审计记录）
15. LLM Prompt 注入防护（用户输入是否直接拼接到 prompt）
16. LLM 输出验证（LLM 响应是否经过清洗后才返回给用户）
17. 工具调用权限控制（Agent 工具是否有沙箱和权限边界）
18. 数据泄露防护（LLM 上下文中是否包含不应暴露的敏感数据）

## 审计方法
- ace-tool: search_context("认证和授权机制的实现") → 理解安全架构
- ace-tool: search_context("敏感信息处理和密钥管理") → 理解密钥管理策略
- ace-tool: search_context("prompt 构建和用户输入处理") → 检查 prompt 注入风险
- exa: web_search("OWASP Top 10 2025 Go") → 最新安全标准
- exa: web_search("OWASP LLM Top 10 prompt injection defense") → LLM 安全最佳实践
- Grep: r.URL.Query().Get("api_key") → query string 敏感参数
- Grep: db.Raw, db.Exec → SQL 拼接风险
- Grep: "key", "password", "token", "secret" → 日志中的敏感字段
- Grep: http.Get, http.Post → SSRF 风险点
- Grep: fmt.Sprintf.*prompt|user.*input.*prompt → prompt 注入风险
- Bash: govulncheck ./... 2>&1 | head -50 → 已知漏洞扫描（如可用）
```

### 维度 8：性能基线 (`perf`)

```
你是性能审计专家。

## 检查项
1. 基准测试覆盖（关键路径是否有 Benchmark）
2. 内存分配热点（循环内 make/append/new、大对象复制）
3. goroutine 泄漏风险（启动的 goroutine 是否都有退出机制）
4. 连接池配置（DB/HTTP/Redis 连接池参数）
5. 缓存策略（缓存层、过期/淘汰机制、穿透防护）
6. JSON 序列化效率
7. sync.Pool 使用（高频分配对象是否池化）
8. context 超时传播（外部调用是否都有超时）
9. Prometheus 指标基数（label 值是否有限）
10. 批处理能力（是否支持批量操作减少 round-trip）
11. 字符串拼接效率（是否在循环中用 + 拼接）
12. 预分配（make 是否指定了合理的 cap）

## 审计方法
- 搜索 Benchmark* 函数
- 搜索 for 循环内的 make(), append(), new()
- 搜索 go func() 启动点，检查退出机制
- 检查连接池配置
- 搜索 sync.Pool 使用
- 检查 context.WithTimeout 分布
```

### 维度 9：并发安全 (`concurrency`)

```
你是并发安全审计专家。

## 检查项
1. 数据竞争风险（共享变量是否有锁保护或使用 atomic）
2. goroutine 生命周期管理（启动 vs 退出是否配对）
3. channel 使用安全（是否有 deadlock 风险、是否 close 后还 send）
4. sync.Mutex/RWMutex 使用正确性（是否有锁嵌套、是否 defer Unlock）
5. sync.WaitGroup 使用正确性（Add 是否在 goroutine 外调用）
6. context 取消传播（是否所有 goroutine 都监听 ctx.Done()）
7. map 并发访问（是否使用 sync.Map 或加锁）
8. 全局变量/单例的并发安全性
9. init() 函数中的副作用（是否有并发初始化风险）
10. select 语句是否有 default 分支防止阻塞

## 审计方法
- ace-tool: search_context("并发控制和锁机制") → 理解并发架构
- ace-tool: search_context("goroutine 生命周期和退出机制") → 理解 goroutine 管理
- Grep: go func() → goroutine 启动点
- Grep: sync.Mutex|sync.RWMutex → 锁使用
- Grep: sync.Map → 并发安全 map
- Grep: chan\s → channel 定义
- Grep: atomic\. → 原子操作
- Grep: var\s+\w+\s+(map|slice|\[\]) → 全局可变变量
- Bash: go vet ./... 2>&1 | grep -i "race\|lock\|mutex" → 静态分析并发问题
```

### 维度 10：错误处理 (`error`)

```
你是错误处理审计专家。

## 检查项
1. 错误是否被静默忽略（_ = someFunc() 或 if err != nil 后无处理）
2. 错误包装是否保留上下文（fmt.Errorf + %w 使用）
3. 错误类型层次是否清晰（sentinel error vs typed error vs wrapped error）
4. panic 使用是否合理（仅用于不可恢复错误，不用于业务逻辑）
5. recover 是否在正确位置（HTTP handler 顶层、goroutine 入口）
6. 错误日志是否包含足够上下文（请求 ID、用户 ID、操作名）
7. 错误响应是否对外隐藏内部细节（不暴露堆栈、SQL、文件路径）
8. 超时错误是否有专门处理（区分 context.DeadlineExceeded vs 其他）
9. 重试逻辑是否区分可重试 vs 不可重试错误
10. 错误码是否有统一的注册/文档机制

## 审计方法
- 搜索 `_ =` 和 `_ :=` 模式，检查被忽略的错误
- 搜索 `if err != nil` 后的处理逻辑
- 搜索 panic() 调用，检查使用场景
- 搜索 recover() 调用，检查位置
- 搜索 fmt.Errorf 中 %w 的使用率
- 检查 HTTP 错误响应是否泄露内部信息
```

### 维度 11：可观测性 (`observability`)

```
你是可观测性审计专家。参考 OpenTelemetry 最新最佳实践。

## 检查项
1. OpenTelemetry 集成完整性（TracerProvider + MeterProvider + LoggerProvider）
2. 分布式追踪覆盖（HTTP 入口 → 业务逻辑 → 外部调用 全链路 span）
3. span 属性丰富度（是否包含关键业务属性）
4. metrics 维度覆盖（RED metrics: Rate/Error/Duration）
5. 自定义 metrics 命名规范（是否遵循 Prometheus/OTel semantic conventions）
6. 结构化日志（是否使用 slog/zap/zerolog，是否有统一字段）
7. 日志级别使用是否合理（Debug/Info/Warn/Error 分级）
8. 告警规则是否定义（Prometheus alerting rules）
9. 追踪采样策略（是否配置了合理的采样率）
10. 健康检查与可观测性的集成（探针是否暴露内部状态）
11. LLM 可观测性指标（llm.request.duration, tokens.input/output, cost, errors）
12. Agent 可观测性指标（agent.execution.duration, tool_calls, state_transitions）
13. RAG 可观测性指标（rag.retrieval.duration, relevance score）
14. Prometheus label 基数控制（避免 user_id, agent_id 等高基数 label）
15. Graceful shutdown 时 flush pending spans/metrics

## 审计方法
- ace-tool: search_context("可观测性和监控指标的实现") → 理解可观测性架构
- ace-tool: search_context("Prometheus metrics 注册和 label 定义") → 检查 label 基数
- Grep: otel, opentelemetry, TracerProvider, MeterProvider → OTel 集成
- Grep: span.Start, span.End, span.SetAttributes → 追踪覆盖
- Grep: prometheus.NewCounter, prometheus.NewHistogram → metrics 注册
- Grep: slog, zap, zerolog → 日志框架
- Grep: request_id, trace_id → 日志关联字段
- exa: web_search("OpenTelemetry Go SDK best practices 2025") → 最新 OTel 实践
- Read: internal/metrics/ → 检查 metrics 定义和 label 基数
```

### 维度 12：配置管理 (`config`)

```
你是配置管理审计专家。

## 检查项
1. 配置结构体是否有完整的默认值
2. 配置验证（必填字段、值范围、格式校验）
3. 敏感配置是否与普通配置分离（密钥不在配置文件中明文存储）
4. 环境变量映射是否完整且有文档
5. 配置热重载是否线程安全
6. 配置变更是否有审计日志
7. 多环境配置管理（dev/staging/prod 差异是否清晰）
8. 配置文件格式一致性（YAML/TOML/JSON 是否统一）
9. 废弃配置项是否有迁移提示
10. 配置示例文件是否与实际结构同步

## 审计方法
- 读取配置结构体定义，检查 default tag 或初始化逻辑
- 搜索 Validate() 方法
- 搜索 os.Getenv 调用，对比配置结构体字段
- 检查 config.example.yaml 与实际结构的差异
- 搜索 viper, koanf 等配置库的使用模式
```

### 维度 13：依赖管理 (`deps`)

```
你是依赖管理审计专家。

## 检查项
1. go.mod 中是否有过时的依赖（major version 落后 2+ 个版本）
2. 是否有已知安全漏洞的依赖
3. replace 指令是否有注释说明原因
4. 间接依赖是否过多（依赖树深度）
5. 是否有功能重叠的依赖（如同时引入多个 HTTP 框架）
6. vendor 目录是否与 go.sum 一致（如使用 vendor）
7. 构建约束（build tags）是否正确使用
8. CGO 依赖是否明确标注
9. 私有模块代理配置（GOPRIVATE/GONOSUMCHECK）
10. 依赖许可证合规性（是否有 GPL 等传染性许可证）

## 审计方法
- exa: web_search("Go dependency vulnerability CVE 2025") → 搜索最新已知漏洞
- exa: get_code_context("go.mod dependency audit best practices") → 依赖审计最佳实践
- Read: go.mod → 分析依赖版本和 replace 指令
- Grep: replace → replace 指令
- Grep: //go:build|// \+build → 构建标签
- Bash: go mod tidy -diff 2>&1 | head -20 → 检查依赖是否整洁
- Bash: go list -m -json all 2>/dev/null | head -100 → 依赖树概览
- Bash: govulncheck ./... 2>&1 | head -50 → 漏洞扫描（如可用）
```

### 维度 14：文档完整性 (`docs`)

```
你是文档完整性审计专家。

## 检查项
1. README 是否包含：项目描述、快速开始、架构概览、API 文档链接
2. 公开接口是否有 godoc 注释
3. 复杂算法/业务逻辑是否有注释说明
4. CHANGELOG 是否维护
5. 部署文档是否完整（环境要求、配置说明、运维手册）
6. API 文档是否与代码同步（OpenAPI spec 更新时间）
7. 架构决策记录（ADR）是否存在
8. 贡献指南（CONTRIBUTING.md）是否存在
9. 错误码文档是否完整
10. 配置项文档是否与代码同步

## 审计方法
- 检查 README.md 的章节完整性
- 统计导出函数/类型的 godoc 覆盖率
- 检查 docs/ 目录结构
- 对比 OpenAPI spec 的最后修改时间与 handler 代码
- 搜索 ADR 或 decision record 文件
```

### 维度 15：数据库与迁移 (`migration`)

```
你是数据库与迁移审计专家。

## 检查项
1. 迁移文件是否有 up/down 配对
2. 迁移是否幂等（重复执行不报错）
3. 迁移是否有回滚测试
4. 数据库连接管理（连接池大小、超时配置、重连机制）
5. 事务使用是否正确（是否有长事务、是否正确 rollback）
6. 索引策略（查询是否有对应索引、是否有冗余索引）
7. 数据库 schema 与 Go 结构体是否同步
8. 软删除 vs 硬删除策略是否一致
9. 数据库超时配置（query timeout, connection timeout）
10. 数据备份和恢复策略

## 审计方法
- 搜索 migration 文件，检查 up/down 配对
- 搜索 db.Begin(), tx.Commit(), tx.Rollback() 模式
- 检查 SQL 索引定义
- 对比 migration schema 与 Go struct 的字段
- 检查数据库连接配置参数
```

### 维度 16：LLM 集成质量 (`llm`)

```
你是 LLM 集成质量审计专家。

## 检查项
1. Provider 接口实现完整性（所有 provider 是否实现了全部方法）
2. API Key 管理安全性（不硬编码、不日志打印、轮换机制）
3. 请求重试策略（指数退避、最大重试次数、可重试错误判断）
4. 超时配置（连接超时、读取超时、总超时）
5. 流式响应处理（SSE 解析、错误处理、连接断开恢复）
6. Token 计数准确性（是否有 tokenizer、是否与 provider 一致）
7. 速率限制处理（429 响应处理、请求队列、优先级）
8. 模型回退策略（主模型不可用时是否有备选）
9. 请求/响应日志（是否记录但脱敏、是否可配置级别）
10. 成本追踪（是否记录 token 使用量、是否有预算控制）
11. Prompt 模板管理（是否有版本控制、是否支持变量替换）
12. 上下文窗口管理（是否有截断策略、是否保留关键信息）

## 审计方法
- ace-tool: search_context("LLM provider 接口实现和 API 调用") → 理解 provider 架构
- ace-tool: search_context("流式响应处理和 SSE 解析") → 理解流式处理
- ace-tool: search_context("token 计数和上下文窗口管理") → 理解 token 管理
- exa: web_search("LLM API best practices rate limiting retry 2025") → 最新最佳实践
- context7: 查询 OpenAI/Anthropic SDK 文档验证 API 用法正确性
- Grep: api.?key|api.?token → API Key 传递方式
- Grep: retry|backoff|exponential → 重试逻辑
- Grep: stream|SSE|event.?source → 流式处理
- Grep: token.*count|tokenize → token 计数
- Grep: 429|rate.?limit|quota → 速率限制处理
- Read: llm/ 目录下各 provider 实现 → 逐一检查完整性
```

### 维度 17：Agent 框架质量 (`agent`)

```
你是 Agent 框架质量审计专家。

## 检查项
1. Agent 生命周期管理（创建 → 初始化 → 运行 → 暂停 → 恢复 → 终止）
2. 工具调用安全性（工具权限控制、输入验证、超时限制）
3. 对话历史管理（内存限制、持久化、压缩策略）
4. 多 Agent 协作机制（消息传递、任务分配、冲突解决）
5. Agent 状态持久化（断点恢复、状态快照）
6. 护栏机制（输入/输出过滤、内容安全、行为约束）
7. 执行沙箱（代码执行隔离、资源限制、超时控制）
8. Agent 可观测性（执行轨迹、决策日志、性能指标）
9. 错误恢复策略（工具调用失败、LLM 响应异常、超时）
10. Agent 注册与发现机制
11. 人机协作接口（Human-in-the-loop 断点、审批流程）
12. Agent 版本管理和热更新

## 审计方法
- ace-tool: search_context("Agent 生命周期管理和状态机") → 理解 Agent 架构
- ace-tool: search_context("工具调用安全性和权限控制") → 理解工具安全
- ace-tool: search_context("多 Agent 协作和消息传递机制") → 理解协作架构
- ace-tool: search_context("Agent 护栏和内容安全过滤") → 理解安全边界
- exa: web_search("AI agent framework security best practices 2025") → 最新 Agent 安全实践
- Grep: guardrail|filter|safety → 护栏机制
- Grep: sandbox|isolat → 沙箱隔离
- Grep: human.?in.?the.?loop|hitl|approval → HITL 机制
- Grep: agent.*register|discover → 注册发现
- Read: agent/ 目录核心接口和实现 → 逐一检查
```

### 维度 18：RAG 系统质量 (`rag`)

```
你是 RAG 系统质量审计专家。

## 检查项
1. 向量存储接口抽象（是否支持多后端切换）
2. 嵌入模型管理（模型选择、维度配置、批量处理）
3. 文档分块策略（分块大小、重叠、语义分块）
4. 检索质量（相似度阈值、top-k 配置、重排序）
5. 索引更新策略（增量 vs 全量、一致性保证）
6. 元数据过滤（是否支持结构化过滤条件）
7. 缓存策略（查询缓存、嵌入缓存）
8. 错误处理（向量库不可用时的降级策略）
9. 数据清洗和预处理管道
10. 检索结果的可解释性（是否返回相似度分数、来源引用）

## 审计方法
- 读取 rag/ 目录结构
- 检查向量存储的接口定义和实现
- 搜索 embedding, chunk, split 相关代码
- 检查检索参数配置
- 搜索缓存相关代码
```

### 维度 19：部署与运维 (`deploy`)

```
你是部署与运维审计专家。

## 检查项
1. Dockerfile 最佳实践（多阶段构建、最小镜像、非 root、.dockerignore）
2. Kubernetes 配置（资源限制、探针、PDB、HPA、安全上下文）
3. Helm Chart 质量（values 文档、模板测试、版本管理）
4. CI/CD 流水线完整性（lint → test → build → deploy 全链路）
5. 环境隔离（dev/staging/prod 配置差异管理）
6. 密钥管理（Kubernetes Secrets、外部密钥管理器集成）
7. 滚动更新策略（maxSurge/maxUnavailable、回滚机制）
8. 日志收集（是否输出到 stdout/stderr、是否有结构化格式）
9. 监控告警（Prometheus rules、Grafana dashboards）
10. 灾难恢复计划（备份策略、RTO/RPO 定义）

## 审计方法
- 检查 Dockerfile 的每一层
- 读取 Kubernetes manifests 或 Helm templates
- 检查 CI/CD 配置文件（.github/workflows, .gitlab-ci.yml 等）
- 搜索 Secret, ConfigMap 的使用方式
- 检查部署策略配置
```

### 维度 20：韧性与容错 (`resilience`)

```
你是韧性与容错审计专家。

## 检查项
1. 熔断器实现（三态状态机、阈值配置、半开探测）
2. 重试策略（指数退避、抖动、最大重试、可重试错误判断）
3. 超时层次（连接超时 < 请求超时 < 操作超时 < 全局超时）
4. 降级策略（核心功能不可用时的备选方案）
5. 背压机制（请求队列、拒绝策略、优先级）
6. 幂等性保证（重复请求是否安全）
7. 分布式锁（如有，实现正确性、超时释放）
8. 健康检查自愈（不健康时是否自动重启/重连）
9. 级联故障防护（一个服务故障是否会拖垮整个系统）
10. 混沌工程就绪度（是否有故障注入接口）

## 审计方法
- 搜索 circuit breaker, breaker 相关代码
- 搜索 retry, backoff, exponential 相关代码
- 搜索 timeout 配置的层次关系
- 检查降级逻辑（fallback, degrade）
- 搜索 semaphore, rate limit, queue 相关代码
- 检查分布式锁的实现（如有 Redis lock）
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
- **P0**：会导致生产事故（数据丢失、安全漏洞、服务不可用、数据竞争）
- **P1**：影响可维护性或一致性（类型不匹配、文档过时、测试缺失、错误处理不当）
- **P2**：改进建议（代码组织、性能优化、最佳实践、文档补充）

置信度标记（交叉验证后）：
- **[高]**：多个维度互相印证的发现
- **[中]**：单一维度发现，但有明确证据
- **[低]**：推测性发现，需要人工确认

---

## 汇总报告格式

所有维度完成后，生成以下汇总：

```markdown
## 生产就绪度深度审计报告

### 项目画像
- 语言/框架: Go 1.24, 自研 Agent 框架
- 规模: XXX Go 文件, XX 个包
- 审计维度: N 个（列出）
- 跳过维度: M 个（列出原因）

### 总评分：X.X / 10

| # | 维度 | 评分 | 状态 | 关键发现数 |
|---|------|------|------|-----------|
| 1 | 生产基础设施 | X/10 | ✅/⚠️/❌ | P0:X P1:X P2:X |
| 2 | API 一致性 | X/10 | ✅/⚠️/❌ | P0:X P1:X P2:X |
| ... | ... | ... | ... | ... |

### 交叉验证发现
- [高置信度] 问题描述（维度A + 维度B 互相印证）
- ...

### P0 问题（必须修复）— 按修复依赖排序
1. [文件:行号] [置信度] 问题描述 → 修复方案
   - 影响范围: [相关文件列表]
   - 修复依赖: 无 / 依赖 #N

### P1 问题（应该修复）
- [文件:行号] [置信度] 问题描述 → 修复方案

### P2 问题（建议改进）
- [文件:行号] [置信度] 问题描述 → 修复方案

### 修复路线图建议
1. 第一批（无依赖的 P0）: [问题列表]
2. 第二批（依赖第一批的 P0 + 独立 P1）: [问题列表]
3. 第三批（剩余 P1 + P2）: [问题列表]
```

---

## 行业标准参考框架

审计维度的设计参考了以下行业标准，审计 agent 在评估时应对照这些标准：

### Google SRE 生产就绪度审查（PRR）
- 服务可靠性：SLO/SLI 定义、错误预算、告警策略
- 容量规划：负载测试、自动扩缩容、资源限制
- 故障响应：Runbook、On-call 流程、事后复盘模板
- 变更管理：渐进式发布、金丝雀部署、回滚机制

### CNCF 云原生安全清单
- 供应链安全：镜像签名、SBOM、依赖扫描
- 运行时安全：最小权限、网络策略、Pod 安全标准
- 可观测性：分布式追踪、结构化日志、指标聚合

### 12-Factor App
- 配置外部化、无状态进程、端口绑定、并发模型
- 日志作为事件流、管理进程、开发/生产一致性

### OWASP LLM Top 10（2025）
- LLM01: Prompt 注入（直接/间接）
- LLM02: 不安全的输出处理
- LLM03: 训练数据投毒
- LLM04: 模型拒绝服务
- LLM05: 供应链漏洞
- LLM06: 敏感信息泄露
- LLM07: 不安全的插件/工具设计
- LLM08: 过度代理权限
- LLM09: 过度依赖 LLM 输出
- LLM10: 模型盗窃

> 审计 agent 可用 `mcp__exa__web_search_exa` 搜索这些标准的最新版本。

---

## 增量审计模式 (`delta`)

如果项目之前已经做过审计（检查 `.trellis/workspace/` 中的历史记录），可以进行增量审计：

1. 读取上次审计的问题清单
2. 仅检查上次标记为 P0/P1 的问题是否已修复
3. 对新增/修改的文件（`git diff --name-only <last-audit-commit>..HEAD`）进行全量审计
4. 对变更文件涉及的维度启动针对性审计（不是全部 20 个维度）
5. 输出增量报告：已修复 / 未修复 / 新发现 / 回归

---

## 反模式（严格避免）

- 问用户"要审计哪些维度？"— 从参数或默认值推导
- 问用户"项目路径在哪？"— 使用当前工作目录
- 给出没有文件引用的模糊评价 — 每个发现必须有 `file:line`
- 评分基于"感觉"而非检查项通过率
- 修复建议是"建议改进"而非具体的代码修改方案
- 串行启动 agent — 必须在同一条消息中并行发送
- 审计已知问题而不先读取项目规范 — 先读 `.trellis/spec/`
- 盲目启动 20 个 agent — 根据项目画像智能决策
- 忽略交叉验证 — 不同维度的发现必须互相对照
- 重复报告同一问题 — 合并去重后再输出

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


# AgentFlow 开发指南

## 项目概况

AgentFlow 是一个生产级 Go 语言 LLM Agent 框架，采用分层架构设计，支持多种 LLM 提供商、记忆系统、推理模式和工作流引擎。

## 构建与测试命令

### 主要构建命令

```bash
# 显示所有可用命令
make help

# 构建二进制文件
make build                    # 构建当前平台的二进制文件
make build-linux             # 构建 Linux 二进制文件
make build-darwin            # 构建 macOS 二进制文件
make build-windows           # 构建 Windows 二进制文件
make build-all               # 构建所有平台二进制文件

# 安装到 GOPATH/bin
make install

# 开发模式运行
make dev                     # 带热重载运行（需要 air）
make run                     # 构建并运行服务

# 依赖管理
make deps                    # 下载依赖
make deps-update             # 更新依赖
make tidy                    # 整理依赖
```

### 测试命令

```bash
# 运行所有单元测试（包含竞态检测和覆盖率）
make test                    # 运行所有测试，包含竞态检测和覆盖率
make test-short              # 运行快速测试（跳过长时间测试）

# 运行单个包测试（最常用的单测方式）
go test ./agent/capabilities/reasoning -v          # 详细模式
go test ./agent/capabilities/reasoning -run TestReflexion  # 运行特定测试函数
go test ./agent/capabilities/reasoning -v -count=1 # 禁用缓存，运行一次

# 运行单个包的特定测试
go test ./agent/capabilities/reasoning -v -run ^TestReflexion$  # 精确匹配测试函数
go test ./agent/capabilities/reasoning -v -run 'Test.*Stream'   # 正则匹配

# 集成和 E2E 测试
make test-integration         # 运行集成测试（需要外部依赖）
make test-e2e                # 运行 E2E 测试（需要 Docker 服务）

# 运行所有测试
make test-all                # 运行单元、集成和 E2E 测试

# 基准测试
make bench                   # 运行基准测试
```

### 覆盖率命令

```bash
# 生成覆盖率报告
make test-cover              # 运行测试并生成覆盖率报告
make coverage                # test-cover 的别名
make coverage-func           # 显示函数级别的覆盖率统计
make coverage-html           # 在浏览器中打开覆盖率报告
make coverage-check          # 检查覆盖率是否达到阈值（默认 55%）
make coverage-badge          # 生成覆盖率徽章数据
```

### 代码质量检查

```bash
# 代码格式化
make fmt                     # 使用 gofmt 格式化代码
make vet                     # 运行 go vet 静态分析
make lint                    # 运行 golangci-lint（如果已安装，否则运行 go vet）
make verify                  # 完整验证（fmt + vet + lint + test）

# 架构守卫检查
make arch-guard              # 架构依赖守卫检查（分层方向/包体积/单文件包）
make arch-guard-ci           # CI 严格架构守卫（warning 按阈值升级为 error）

# 重构计划检查
make refactor-plan-lint      # 重构计划格式检查（必须包含 [ ]/[x] 状态与核心章节）
make refactor-plan-report    # 输出重构计划进度汇总
make refactor-plan-gate      # 重构计划收尾门禁（存在 [ ] 则失败）

# 文档检查
make docs-lint               # 检查文档中的死链接
make docs-api-drift          # 检查文档中是否残留已废弃的 API 引用
make docs-surface-check      # 检查 README/docs/examples 的官方/legacy 产品面口径一致性
make docs-examples-check     # 提取文档中 Go 代码块做编译检查
```

### Docker 命令

```bash
# Docker 构建与运行
make docker-build            # 构建 Docker 镜像
make docker-run              # 运行 Docker 容器
make docker-push             # 推送 Docker 镜像到仓库

# Docker Compose 开发环境
make up                      # 启动本地开发环境
make up-build                # 重新构建并启动本地环境
make up-monitoring           # 启动带监控的本地环境
make down                    # 停止本地环境
make down-v                  # 停止本地环境并删除数据卷
make logs                    # 查看服务日志
make ps                      # 查看服务状态
make restart                 # 重启本地环境

# 清理
make clean                   # 清理构建产物
make clean-docker            # 清理 Docker 资源
make clean-all               # 清理所有资源
```

### 数据库迁移

```bash
make migrate-up              # 运行所有待执行的数据库迁移
make migrate-down            # 回滚最后一次数据库迁移
make migrate-status          # 显示数据库迁移状态
```

### 文档生成

```bash
make docs                    # 生成 API 文档（swagger 的别名）
make docs-swagger            # 生成 Swagger/OpenAPI 文档
make docs-serve              # 启动 Swagger UI 服务器
make docs-validate           # 验证 OpenAPI 规范
make docs-generate-client    # 从 OpenAPI 生成客户端代码
make install-swag            # 安装 swag 工具
```

## 代码风格指南

### Go 语言规范

#### 导入分组与排序
```go
import (
    "context"
    "fmt"
    "strings"
    "time"

    "github.com/BaSui01/agentflow/types"
    
    "github.com/BaSui01/agentflow/llm/capabilities/tools"
    llmcore "github.com/BaSui01/agentflow/llm/core"
    "go.uber.org/zap"
)
```
- 标准库导入在前，空行分隔
- 第三方库在后
- 项目内部包按层级排序
- 使用别名避免冲突（如 `llmcore`）

#### 命名约定
- **包名**：小写，无下划线，单数形式
- **接口名**：`er` 结尾（如 `ChatRequestAdapter`）
- **函数名**：驼峰式，首字母大写导出
- **变量名**：驼峰式，首字母小写私有
- **常量**：全部大写，下划线分隔（如 `ErrInputValidation`）
- **错误变量**：`Err` 前缀（如 `ErrInputValidation`）

#### 错误处理
```go
// 错误类型定义
func NewError(code ErrorCode, msg string) *Error {
    return &Error{
        Code:    code,
        Message: msg,
    }
}

// 错误检查
if err != nil {
    return fmt.Errorf("failed to build chat request: %w", err)
}

// 错误包装
if len(messages) == 0 {
    return nil, types.NewError(types.ErrInputValidation, "messages cannot be nil or empty")
}
```

#### 类型与结构体
```go
// Config 结构体
type ReflexionConfig struct {
    MaxTrials        int           `json:"max_trials"`
    SuccessThreshold float64       `json:"success_threshold"`
    Timeout          time.Duration `json:"timeout"`
    EnableMemory     bool          `json:"enable_memory"`
    Model            string        `json:"model,omitempty"`
}

// 构造函数
func DefaultReflexionConfig() ReflexionConfig {
    return ReflexionConfig{
        MaxTrials:        5,
        SuccessThreshold: 0.8,
        Timeout:          300 * time.Second,
        EnableMemory:     true,
        Model:            "gpt-4o",
    }
}
```

#### 注释规范
- **导出类型/函数**：必须有 GoDoc 注释
- **复杂逻辑**：解释意图和算法
- **TODO/FIXME**：使用标准标记
- **中文注释**：允许中文注释，但主要用英文

### Lint 配置

项目使用 `golangci-lint` 配置（见 `.golangci.yml`）：

#### 强制检查项
1. **govet**：Go 官方静态分析
2. **staticcheck**：高级静态分析
3. **errcheck**：检查未处理的错误返回值（强制）
4. **gosimple**：代码简化建议
5. **ineffassign**：检测无效赋值
6. **unused**：检测未使用的代码
7. **typecheck**：类型检查

#### 风格检查
- **gocritic**：代码风格和性能检查
- **gofmt**：格式化检查
- **goimports**：import 排序检查
- **misspell**：拼写检查

#### 安全与复杂度
- **gosec**：安全漏洞检查（SQL注入、硬编码密钥等）
- **gocyclo**：圈复杂度检查（阈值：15）
- **gocognit**：认知复杂度检查（阈值：20）

#### 最佳实践
- **nolintlint**：检查 nolint 指令规范性
- **exportloopref**：检查循环变量导出问题
- **prealloc**：建议预分配切片容量
- **unconvert**：检测不必要的类型转换
- **unparam**：检测未使用的函数参数
- **nakedret**：检查裸返回语句（函数体超过 30 行禁止）
- **bodyclose**：检查 HTTP response body 是否关闭

### 架构分层（强制）

项目采用严格的分层架构，依赖方向必须遵守：

```
Layer 0 types/      - 零依赖核心类型层，只允许被依赖
Layer 1 llm/        - Provider 抽象与实现层，不得依赖 agent/、workflow/、api/、cmd/
Layer 2 agent/ + rag/ - 核心能力层，可依赖 llm/ 与 types/，不得依赖 cmd/
Layer 3 workflow/   - 编排层，可依赖 agent/、rag/、llm/、types/
适配层 api/         - 仅做协议转换与入站/出站适配，不承载核心业务决策
组合根 cmd/         - 只做启动装配、生命周期管理、配置注入；不下沉业务实现
基础设施层 pkg/     - 不得反向依赖 api/ 与 cmd/
```

### 开发规则（强制）

#### 1) 禁止兼容代码
- **禁止编写兼容代码**：代码修改时不允许为兼容旧逻辑保留分支、兜底或双实现
- **只保留单一实现**：必须删除被替代的旧实现，只保留修改后唯一且最正确的实现
- **禁止双轨迁移**：不允许"新老逻辑并存一段时间再删"的方案，除非明确有迁移任务文档并单独批准

#### 2) 服务启动链路
保持单入口：
```
cmd/agentflow/main.go 
→ internal/app/bootstrap 
→ cmd/agentflow/server_* 
→ api/routes 
→ api/handlers 
→ domain(agent/rag/workflow/llm)
```

#### 3) 代码复用与简洁调用
- **复用优先**：新增能力前先复用现有 `builder/factory/adapter`，禁止重复造轮子
- **API 简洁**：对外入口优先保持少量稳定入口，避免新增并行入口
- **单一职责**：文件和包职责必须清晰，避免"God Object / God Package"
- **命名可检索**：模块命名与目录结构要直观表达职责，便于快速定位与调用

#### 4) 变更与校验
- 所有架构相关改动必须同步更新对应文档
- 提交前必须通过架构守卫（如 `architecture_guard_test.go`、`scripts/arch_guard.ps1`）
- 如确需突破架构规则，必须先提交 ADR 或架构变更说明，再实施代码改动

### 测试规范

#### 测试文件命名
- 单元测试：`*_test.go`
- 集成测试：`*_integration_test.go`
- E2E 测试：`*_e2e_test.go`

#### 测试标签
```go
//go:build integration
// +build integration

package integration_test

import "testing"

func TestIntegrationSomething(t *testing.T) {
    // 需要外部依赖的测试
}
```

#### 测试覆盖率要求
- **最低要求**：55% 整体覆盖率
- **关键路径**：核心业务逻辑要求 80%+ 覆盖率
- **测试运行**：使用 `go test -race -cover` 运行测试

### 测试建议
- **Goroutine 泄漏检测**：建议在关键包的 `TestMain` 中集成 `go.uber.org/goleak` 的 `VerifyTestMain`
- **禁止擅自恢复文件**：未获得用户明确授权，不允许以 `git checkout`、`git show HEAD > file`、覆盖写回等方式恢复、回滚或重置任何工作区文件
- **测试范围最小化**：默认只运行与当前修改直接相关的测试、构建或校验；不要擅自扩大到无关模块、全量测试或全仓回归，除非用户明确要求

## Git 工作流

### 提交消息格式
```
类型(范围): 简要描述

详细描述（可选）

- 变更点 1
- 变更点 2

关联 Issue: #123
```

**类型**：
- `feat`：新功能
- `fix`：bug 修复
- `docs`：文档更新
- `style`：代码样式调整（不影响功能）
- `refactor`：重构（不新增功能，不修复 bug）
- `perf`：性能优化
- `test`：测试相关
- `chore`：构建过程或辅助工具的变动

### 分支策略
- `main`：主分支，受保护
- `feature/*`：功能分支
- `bugfix/*`：bug 修复分支
- `hotfix/*`：热修复分支

## CI/CD 流程

GitHub Actions 自动运行以下检查：

1. **代码质量**：`golangci-lint`、`go vet`
2. **架构守卫**：`scripts/arch_guard.ps1`
3. **测试**：单元测试 + 集成测试（如有标签）
4. **覆盖率**：检查 55% 阈值
5. **文档检查**：API 文档一致性检查
6. **跨平台构建**：Linux、macOS、Windows
7. **安全扫描**：`govulncheck`

## 模型厂商命名规范

### 中文命名基线统一收口到文档
涉及模型厂商、产品线、模型 ID、latest 表述时，以 `docs/cn/guides/模型厂商与模型中文命名规范.md` 为准。

### 代码名与模型 ID 不翻译
目录名、配置键、环境变量、Provider code、API model id 必须保留原始英文，例如：
- `anthropic`
- `grok`
- `gemini`
- `gpt-5.4`

### 中文文档首提写"品牌/产品 + 英文代码/ID"
如 `Anthropic Claude（anthropic）`、`xAI Grok（grok）`、`Google Gemini（gemini）`、`通义千问 Qwen（qwen）`

### "最新模型"必须带绝对日期和官方来源
禁止写没有日期的"当前最新 / latest / 主推模型"；至少注明"截至 YYYY-MM-DD"

## 外部参考目录

### `CC-Source/` 与 `docs/claude-code/` 
仅作外部参考学习资料，用于借鉴设计与实现思路，不属于当前项目正式实现。

### 默认排除主项目语境
做当前项目设计、开发、评审、文档同步、架构守卫判断时，默认排除上述目录；仅在明确要求参考外部实现时再读取或引用。

## 交互语言规范

### 默认使用中文交互
在本仓库作用域内，与用户沟通、汇报进展、总结结果时默认使用中文。

### 专业术语可保留原文
专业术语、代码、命令、路径、标识符、原始报错与协议字段可直接保留英文或原文，避免歧义。

## Agent 框架官方入口

### 仓库级正式入口
```go
sdk.New(opts).Build(ctx)
```

### 单 Agent 正式入口
```go
agent/runtime
```

### 多 Agent 正式入口
```go
agent/team
```

### 显式编排正式入口
```go
workflow/runtime
```

### 统一授权入口
```go
internal/usecase/authorization_service.go
```

## 开发工作流（Trellis）

### 启动会话
```bash
# 使用 /trellis:start 命令开始新会话
# 或使用 start skill
$start
```

### 任务工作流
1. **Brainstorm** → **Task Workflow** 复杂任务
2. **Quick confirm** → **Task Workflow** 简单任务
3. **Direct Edit** 直接编辑（小修改）

### 代码规范检查
```bash
# 提交前运行
$finish-work
```

## 常见问题

### Q: 如何运行单个测试？
A: 使用 `go test ./path/to/package -v -run TestFunctionName`

### Q: 如何检查代码风格问题？
A: 使用 `make lint` 或 `make verify`

### Q: 如何查看项目架构依赖关系？
A: 运行 `make arch-guard`

### Q: 需要突破架构规则怎么办？
A: 必须先提交 ADR 或架构变更说明到 `docs/architecture/ADRs/`，再实施代码改动

### Q: 如何添加新的 Provider？
A: 遵循 `llm/providers/` 目录下的模式，确保不违反分层架构

### Q: 测试覆盖率太低怎么办？
A: 运行 `make coverage-html` 查看覆盖率报告，补充缺失的测试用例

---

**最后更新**: 2025-04-27  
**项目版本**: Go 1.24+  
**最低覆盖率**: 55%
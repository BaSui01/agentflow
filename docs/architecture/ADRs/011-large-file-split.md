# ADR-011: 大文件拆分（>800行）

## 状态
提议中

## 背景
以下文件超过 800 行，建议按职责拆分：

| 文件 | 行数 | 拆分建议 |
|---|---|---|
| `agent/team/internal/engines/multiagent/multi_agent.go` | 1495 | 拆出 `multi_agent_config.go`、`multi_agent_execution.go`、`multi_agent_result.go` |
| `agent/capabilities/tools/registry.go` | 1087 | 拆出 `registry_builtins.go`、`registry_lookup.go` |
| `agent/execution/protocol/a2a/server.go` | 1075 | 拆出 `server_handler.go`、`server_transport.go` |
| `agent/integration/lsp/server.go` | 1044 | 拆出 `server_handler.go`、`server_completion.go` |
| `agent/capabilities/streaming/bidirectional.go` | 768 | 接近阈值，暂不拆分 |

## 决策
将上述文件按职责拆分为同包内的多个文件，不改变包名和导入路径。

## 影响范围
- 同包内拆分，不影响任何外部导入
- 零回归风险

## 执行计划
1. 创建 feature 分支 `refactor/large-file-split`
2. 按优先级逐个拆分
3. 每次拆分后运行该包测试

## 风险
- 风险极低，同包内文件拆分不影响编译和导入

# 🚀 AgentFlow 部署指南

欢迎使用 AgentFlow！本文档提供完整的部署指南，帮助你在各种环境中部署 AgentFlow。

## 📋 目录

- [快速开始](#快速开始)
- [部署选项](#部署选项)
- [基础设施上线清单](#基础设施上线清单)
- [配置说明](#配置说明)
- [环境变量](#环境变量)
- [健康检查](#健康检查)
- [监控和指标](#监控和指标)
- [故障排除](#故障排除)

## 快速开始

### 使用 Docker Compose（推荐用于开发）

```bash
# 克隆仓库
git clone https://github.com/BaSui01/agentflow.git
cd agentflow

# 启动所有服务
docker-compose up -d

# 检查服务状态
docker-compose ps

# 查看日志
docker-compose logs -f agentflow

# 健康检查
curl http://localhost:8080/health
```

### 使用 Docker

```bash
# 构建镜像
docker build -t agentflow:latest .

# 运行容器
docker run -d \
  --name agentflow \
  -p 8080:8080 \
  -p 9091:9091 \
  -e AGENTFLOW_SERVER_METRICS_BIND_ADDRESS=0.0.0.0 \
  -e AGENTFLOW_LLM_API_KEY=your_api_key \
  agentflow:latest
```

### 使用 Helm（Kubernetes）

```bash
# 创建运行时 Secret（至少包含 LLM 凭据；生产环境建议同时提供 JWT）
kubectl create secret generic agentflow-secrets \
  --namespace agentflow \
  --from-literal=AGENTFLOW_LLM_API_KEY=your_api_key \
  --from-literal=AGENTFLOW_SERVER_JWT_SECRET=replace-with-32-byte-secret

# 安装 AgentFlow
helm install agentflow ./deployments/helm/agentflow \
  --namespace agentflow \
  --create-namespace \
  -f ./deployments/helm/agentflow/values-production.yaml \
  --set image.repository=your-registry/agentflow \
  --set image.tag=v1.0.0 \
  --set secrets.existingSecret=agentflow-secrets

# 检查部署状态
kubectl get pods -l app.kubernetes.io/name=agentflow
```

## 部署选项

| 方式 | 适用场景 | 复杂度 | 文档 |
|------|----------|--------|------|
| Docker Compose | 本地开发、测试 | ⭐ | [docker.md](./docker.md) |
| Docker | 单机部署 | ⭐⭐ | [docker.md](./docker.md) |
| Kubernetes + Helm | 生产环境 | ⭐⭐⭐ | [kubernetes.md](./kubernetes.md) |

## 基础设施上线清单

- 基础设施化上线前请先完成并复核 [基础设施上线清单](../archive/基础设施上线清单.md)
- 本仓库当前运行时只暴露 `HTTP(8080)` 与独立 `metrics(9091)` 服务，不提供独立 `gRPC` 服务端口
- `metrics` 默认仅绑定 loopback，若需要容器外抓取必须显式设置 `AGENTFLOW_SERVER_METRICS_BIND_ADDRESS=0.0.0.0`
- `pprof` 默认关闭，只有显式开启 `AGENTFLOW_SERVER_ENABLE_PPROF=true` 才会注册 `/debug/pprof/*`
- Helm Chart 已落库于 `deployments/helm/agentflow/`；chart 默认按生产口径设置 `server.environment=production` 与 `server.allow_no_auth=false`
- chart 通过挂载 `/app/config/config.yaml` 注入 YAML 配置，再由 Secret 覆盖密码、LLM 凭据与 JWT 等敏感值

## 配置说明

AgentFlow 支持多种配置方式，优先级从低到高：

1. **默认值** - 内置的合理默认配置
2. **YAML 文件** - 通过 `--config` 参数指定
3. **环境变量** - 以 `AGENTFLOW_` 为前缀

### 配置文件示例

```yaml
server:
  http_port: 8080
  metrics_port: 9091
  metrics_bind_address: "127.0.0.1"
  enable_pprof: false

agent:
  name: "my-agent"
  model: "gpt-4"
  max_iterations: 10
  temperature: 0.7

redis:
  addr: "redis:6379"

log:
  level: "info"
  format: "json"
```

当前仓库未提供独立 `config.example.yaml`。部署时请参考：

- [docker-compose.yml](/E:/code/agentflow/docker-compose.yml)
- [deployments/helm/agentflow/values.yaml](/E:/code/agentflow/deployments/helm/agentflow/values.yaml)
- [deployments/helm/agentflow/values-production.yaml](/E:/code/agentflow/deployments/helm/agentflow/values-production.yaml)
- 本文档中的配置片段
- [生产环境最佳实践](./production.md)

## 环境变量

所有配置项都可以通过环境变量覆盖，格式为 `AGENTFLOW_<SECTION>_<KEY>`：

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `AGENTFLOW_SERVER_HTTP_PORT` | HTTP 端口 | 8080 |
| `AGENTFLOW_SERVER_METRICS_PORT` | 指标端口 | 9091 |
| `AGENTFLOW_SERVER_METRICS_BIND_ADDRESS` | 指标服务监听地址 | `127.0.0.1` |
| `AGENTFLOW_SERVER_ENABLE_PPROF` | 是否启用 pprof | `false` |
| `AGENTFLOW_AGENT_MODEL` | 默认模型 | gpt-4 |
| `AGENTFLOW_AGENT_MAX_ITERATIONS` | 最大迭代次数 | 10 |
| `AGENTFLOW_REDIS_ADDR` | Redis 地址 | localhost:6379 |
| `AGENTFLOW_DATABASE_HOST` | 数据库主机 | localhost |
| `AGENTFLOW_LLM_API_KEY` | LLM API Key | - |
| `AGENTFLOW_LOG_LEVEL` | 日志级别 | info |

## 健康检查

AgentFlow 提供以下健康检查端点：

| 端点 | 说明 | 用途 |
|------|------|------|
| `/health` | 轻量存活检查，不探测外部依赖 | Kubernetes liveness probe |
| `/healthz` | 轻量存活检查（别名） | 兼容性 |
| `/ready` | 依赖就绪检查，会执行已注册健康检查 | Kubernetes readiness probe |
| `/readyz` | 依赖就绪检查（别名） | 兼容性 |

### 示例响应

```json
{
  "status": "healthy",
  "timestamp": "2026-03-22T10:30:00Z",
  "version": "1.8.10"
}
```

## 监控和指标

### Prometheus 指标与 pprof

AgentFlow 在 `/metrics` 端点（默认端口 `9091`）暴露 Prometheus 指标。默认只绑定 `127.0.0.1`；若需要在容器外、节点外或由外部 Prometheus 抓取，必须显式设置：

```bash
AGENTFLOW_SERVER_METRICS_BIND_ADDRESS=0.0.0.0
```

`pprof` 默认关闭。仅在显式启用后，才会在 metrics 端口注册 `/debug/pprof/*`：

```bash
AGENTFLOW_SERVER_ENABLE_PPROF=true
```

示例：

```bash
curl http://localhost:9091/metrics
```

若使用 Helm，请优先通过以下开关暴露观测面：

```yaml
metrics:
  service:
    enabled: true

serviceMonitor:
  enabled: true
```

这两项任一启用时，chart 会自动把 `AGENTFLOW_SERVER_METRICS_BIND_ADDRESS` 设为 `0.0.0.0`，避免出现“Service/ServiceMonitor 已创建但进程仍只监听 loopback”的不一致。

### 主要指标

| 指标名称 | 类型 | 说明 |
|----------|------|------|
| `agentflow_requests_total` | Counter | 请求总数 |
| `agentflow_request_duration_seconds` | Histogram | 请求延迟 |
| `agentflow_llm_tokens_total` | Counter | LLM Token 使用量 |
| `agentflow_agent_iterations_total` | Counter | Agent 迭代次数 |

### Grafana 仪表盘

使用 Docker Compose 启动监控：

```bash
docker-compose --profile monitoring up -d
```

访问 Grafana：http://localhost:3000（默认账号：admin/admin）

## 故障排除

### 常见问题

#### 1. 服务无法启动

```bash
# 检查日志
docker-compose logs agentflow

# 检查配置
docker-compose exec agentflow cat /app/config/config.yaml
```

#### 2. 无法连接 Redis

```bash
# 检查 Redis 状态
docker-compose exec redis redis-cli ping

# 检查网络连接
docker-compose exec agentflow ping redis
```

#### 3. LLM API 调用失败

- 检查 `AGENTFLOW_LLM_API_KEY` 是否正确设置
- 检查网络是否能访问 LLM API 端点
- 查看日志中的详细错误信息

### 获取帮助

- 📖 [完整文档](https://github.com/BaSui01/agentflow)
- 🐛 [报告问题](https://github.com/BaSui01/agentflow/issues)
- 💬 [讨论区](https://github.com/BaSui01/agentflow/discussions)

## 下一步

- [Docker 部署详解](./docker.md)
- [Kubernetes 部署详解](./kubernetes.md)
- [生产环境最佳实践](./production.md)
- [基础设施上线清单](../archive/基础设施上线清单.md)

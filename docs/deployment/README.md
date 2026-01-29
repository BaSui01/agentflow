# 🚀 AgentFlow 部署指南

欢迎使用 AgentFlow！本文档提供完整的部署指南，帮助你在各种环境中部署 AgentFlow。

## 📋 目录

- [快速开始](#快速开始)
- [部署选项](#部署选项)
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
  -p 9090:9090 \
  -p 9091:9091 \
  -e AGENTFLOW_LLM_API_KEY=your_api_key \
  agentflow:latest
```

### 使用 Helm（Kubernetes）

```bash
# 添加依赖仓库
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# 安装 AgentFlow
helm install agentflow ./deployments/helm/agentflow \
  --set secrets.llmApiKey=your_api_key

# 检查部署状态
kubectl get pods -l app.kubernetes.io/name=agentflow
```

## 部署选项

| 方式 | 适用场景 | 复杂度 | 文档 |
|------|----------|--------|------|
| Docker Compose | 本地开发、测试 | ⭐ | [docker.md](./docker.md) |
| Docker | 单机部署 | ⭐⭐ | [docker.md](./docker.md) |
| Kubernetes + Helm | 生产环境 | ⭐⭐⭐ | [kubernetes.md](./kubernetes.md) |

## 配置说明

AgentFlow 支持多种配置方式，优先级从低到高：

1. **默认值** - 内置的合理默认配置
2. **YAML 文件** - 通过 `--config` 参数指定
3. **环境变量** - 以 `AGENTFLOW_` 为前缀

### 配置文件示例

```yaml
server:
  http_port: 8080
  grpc_port: 9090
  metrics_port: 9091

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

完整配置示例请参考 [`deployments/docker/config.example.yaml`](../docker/config.example.yaml)

## 环境变量

所有配置项都可以通过环境变量覆盖，格式为 `AGENTFLOW_<SECTION>_<KEY>`：

| 环境变量 | 说明 | 默认值 |
|----------|------|--------|
| `AGENTFLOW_SERVER_HTTP_PORT` | HTTP 端口 | 8080 |
| `AGENTFLOW_SERVER_GRPC_PORT` | gRPC 端口 | 9090 |
| `AGENTFLOW_SERVER_METRICS_PORT` | 指标端口 | 9091 |
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
| `/health` | 存活检查 | Kubernetes liveness probe |
| `/healthz` | 存活检查（别名） | 兼容性 |
| `/ready` | 就绪检查 | Kubernetes readiness probe |
| `/readyz` | 就绪检查（别名） | 兼容性 |

### 示例响应

```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "1.0.0"
}
```

## 监控和指标

### Prometheus 指标

AgentFlow 在 `/metrics` 端点（默认端口 9091）暴露 Prometheus 指标：

```bash
curl http://localhost:9091/metrics
```

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

# 🐳 Docker 部署指南

本文档详细介绍如何使用 Docker 和 Docker Compose 部署 AgentFlow。

## 📋 目录

- [前置要求](#前置要求)
- [Docker Compose 部署](#docker-compose-部署)
- [单独 Docker 部署](#单独-docker-部署)
- [构建镜像](#构建镜像)
- [配置说明](#配置说明)
- [数据持久化](#数据持久化)
- [网络配置](#网络配置)
- [常见问题](#常见问题)

## 前置要求

- Docker 20.10+
- Docker Compose 2.0+（使用 Compose 部署时）
- 至少 2GB 可用内存
- 至少 10GB 可用磁盘空间

### 验证安装

```bash
docker --version
docker-compose --version
```

## Docker Compose 部署

### 快速启动

```bash
# 进入项目目录
cd agentflow

# 启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps
```

### 服务说明

| 服务 | 端口 | 说明 |
|------|------|------|
| agentflow | 8080, 9090, 9091 | 主服务 |
| redis | 6379 | 短期记忆缓存 |
| postgres | 5432 | 元数据存储 |
| qdrant | 6333, 6334 | 向量存储 |

### 启动带监控的环境

```bash
# 启动包含 Prometheus 和 Grafana 的完整环境
docker-compose --profile monitoring up -d
```

监控服务：
- Prometheus: http://localhost:9092
- Grafana: http://localhost:3000（admin/admin）

### 常用命令

```bash
# 查看日志
docker-compose logs -f agentflow

# 重启服务
docker-compose restart agentflow

# 停止所有服务
docker-compose down

# 停止并删除数据卷
docker-compose down -v

# 重新构建并启动
docker-compose up -d --build
```

## 单独 Docker 部署

### 运行预构建镜像

```bash
docker run -d \
  --name agentflow \
  -p 8080:8080 \
  -p 9090:9090 \
  -p 9091:9091 \
  -e AGENTFLOW_LLM_API_KEY=your_api_key \
  -e AGENTFLOW_REDIS_ADDR=your_redis:6379 \
  agentflow:latest
```

### 使用配置文件

```bash
# 创建配置目录
mkdir -p ./config

# 复制示例配置
cp deployments/docker/config.example.yaml ./config/config.yaml

# 编辑配置
vim ./config/config.yaml

# 运行容器
docker run -d \
  --name agentflow \
  -p 8080:8080 \
  -p 9090:9090 \
  -p 9091:9091 \
  -v $(pwd)/config:/app/config:ro \
  -e AGENTFLOW_LLM_API_KEY=your_api_key \
  agentflow:latest
```

### 连接外部服务

```bash
docker run -d \
  --name agentflow \
  -p 8080:8080 \
  -p 9090:9090 \
  -p 9091:9091 \
  -e AGENTFLOW_LLM_API_KEY=your_api_key \
  -e AGENTFLOW_REDIS_ADDR=redis.example.com:6379 \
  -e AGENTFLOW_DATABASE_HOST=postgres.example.com \
  -e AGENTFLOW_DATABASE_USER=agentflow \
  -e AGENTFLOW_DATABASE_PASSWORD=secret \
  -e AGENTFLOW_QDRANT_HOST=qdrant.example.com \
  agentflow:latest
```

## 构建镜像

### 基本构建

```bash
docker build -t agentflow:latest .
```

### 带版本信息构建

```bash
docker build \
  --build-arg VERSION=v1.0.0 \
  --build-arg BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  -t agentflow:v1.0.0 \
  .
```

### 多平台构建

```bash
# 创建 buildx builder
docker buildx create --name multiarch --use

# 构建多平台镜像
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t agentflow:latest \
  --push \
  .
```

## 配置说明

### 环境变量

```bash
# 服务器配置
AGENTFLOW_SERVER_HTTP_PORT=8080
AGENTFLOW_SERVER_GRPC_PORT=9090
AGENTFLOW_SERVER_METRICS_PORT=9091

# Agent 配置
AGENTFLOW_AGENT_MODEL=gpt-4
AGENTFLOW_AGENT_MAX_ITERATIONS=10
AGENTFLOW_AGENT_TEMPERATURE=0.7

# Redis 配置
AGENTFLOW_REDIS_ADDR=redis:6379
AGENTFLOW_REDIS_PASSWORD=
AGENTFLOW_REDIS_DB=0

# 数据库配置
AGENTFLOW_DATABASE_HOST=postgres
AGENTFLOW_DATABASE_PORT=5432
AGENTFLOW_DATABASE_USER=agentflow
AGENTFLOW_DATABASE_PASSWORD=secret
AGENTFLOW_DATABASE_NAME=agentflow

# LLM 配置
AGENTFLOW_LLM_API_KEY=your_api_key
AGENTFLOW_LLM_DEFAULT_PROVIDER=openai

# 日志配置
AGENTFLOW_LOG_LEVEL=info
AGENTFLOW_LOG_FORMAT=json
```

### 配置文件挂载

```yaml
# docker-compose.override.yaml
version: "3.9"
services:
  agentflow:
    volumes:
      - ./my-config.yaml:/app/config/config.yaml:ro
```

## 数据持久化

### 数据卷

```yaml
volumes:
  agentflow_data:    # 应用数据
  redis_data:        # Redis 数据
  postgres_data:     # PostgreSQL 数据
  qdrant_data:       # Qdrant 向量数据
```

### 备份数据

```bash
# 备份 PostgreSQL
docker-compose exec postgres pg_dump -U agentflow agentflow > backup.sql

# 备份 Redis
docker-compose exec redis redis-cli BGSAVE

# 备份数据卷
docker run --rm \
  -v agentflow_postgres_data:/data \
  -v $(pwd)/backup:/backup \
  alpine tar czf /backup/postgres_data.tar.gz /data
```

### 恢复数据

```bash
# 恢复 PostgreSQL
cat backup.sql | docker-compose exec -T postgres psql -U agentflow agentflow

# 恢复数据卷
docker run --rm \
  -v agentflow_postgres_data:/data \
  -v $(pwd)/backup:/backup \
  alpine tar xzf /backup/postgres_data.tar.gz -C /
```

## 网络配置

### 默认网络

Docker Compose 创建名为 `agentflow-network` 的桥接网络，所有服务都连接到此网络。

### 自定义网络

```yaml
# docker-compose.override.yaml
networks:
  agentflow-network:
    driver: bridge
    ipam:
      config:
        - subnet: 172.28.0.0/16
```

### 连接外部网络

```yaml
# docker-compose.override.yaml
networks:
  default:
    external:
      name: my-existing-network
```

## 常见问题

### 1. 容器无法启动

```bash
# 检查日志
docker-compose logs agentflow

# 检查资源
docker stats

# 检查磁盘空间
df -h
```

### 2. 无法连接 Redis

```bash
# 检查 Redis 是否运行
docker-compose ps redis

# 测试连接
docker-compose exec redis redis-cli ping

# 检查网络
docker network inspect agentflow-network
```

### 3. 数据库连接失败

```bash
# 检查 PostgreSQL 状态
docker-compose exec postgres pg_isready

# 检查连接
docker-compose exec postgres psql -U agentflow -c "SELECT 1"
```

### 4. 内存不足

```yaml
# docker-compose.override.yaml
services:
  agentflow:
    deploy:
      resources:
        limits:
          memory: 2G
        reservations:
          memory: 512M
```

### 5. 端口冲突

```bash
# 检查端口占用
netstat -tlnp | grep 8080

# 修改端口映射
# docker-compose.override.yaml
services:
  agentflow:
    ports:
      - "8081:8080"  # 使用 8081 替代 8080
```

## 下一步

- [Kubernetes 部署](./kubernetes.md)
- [生产环境最佳实践](./production.md)
- [监控和告警配置](./monitoring.md)

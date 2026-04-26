# ☸️ Kubernetes 部署指南

本文档详细介绍如何使用 Helm 在 Kubernetes 上部署 AgentFlow。

## 📋 目录

- [前置要求](#前置要求)
- [快速开始](#快速开始)
- [Helm Chart 配置](#helm-chart-配置)
- [高可用部署](#高可用部署)
- [安全配置](#安全配置)
- [监控集成](#监控集成)
- [升级和回滚](#升级和回滚)
- [故障排除](#故障排除)

## 前置要求

- Kubernetes 1.24+
- Helm 3.10+
- kubectl 配置正确
- 集群有足够的资源（建议至少 4GB 内存）

### 验证环境

```bash
kubectl version
helm version
kubectl cluster-info
```

## 快速开始

### 1. 添加依赖仓库

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update
```

### 2. 创建命名空间

```bash
kubectl create namespace agentflow
```

### 3. 创建 Secret（API Key）

```bash
# 基础配置（必需）
kubectl create secret generic agentflow-secrets \
  --namespace agentflow \
  --from-literal=AGENTFLOW_LLM_API_KEY=your_api_key_here \
  --from-literal=AGENTFLOW_SERVER_JWT_SECRET=replace-with-32-byte-secret
```

#### Secret 环境变量键完整列表

| 环境变量键 | 说明 | 必需 |
|-----------|------|------|
| `AGENTFLOW_LLM_API_KEY` | 主 LLM API Key（OpenAI 等） | 是 |
| `AGENTFLOW_SERVER_JWT_SECRET` | JWT HMAC 签名密钥（32 字节） | 二选一 |
| `AGENTFLOW_SERVER_JWT_PUBLIC_KEY` | JWT RSA 公钥（PEM 格式） | 二选一 |
| `AGENTFLOW_DATABASE_PASSWORD` | PostgreSQL 密码 | 使用数据库时 |
| `AGENTFLOW_REDIS_PASSWORD` | Redis 密码 | 使用 Redis 时 |
| `AGENTFLOW_MONGODB_PASSWORD` | MongoDB 密码 | 使用 MongoDB 时 |

#### 多模态提供商密钥

启用多模态功能时，需配置对应提供商的 API Key：

| 环境变量键 | 提供商 | 类型 |
|-----------|--------|------|
| `AGENTFLOW_MULTIMODAL_IMAGE_OPENAI_API_KEY` | OpenAI DALL-E | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_GEMINI_API_KEY` | Google Gemini | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_FLUX_API_KEY` | Flux | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_STABILITY_API_KEY` | Stability AI | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_IDEOGRAM_API_KEY` | Ideogram | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_TONGYI_API_KEY` | 通义万象 | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_ZHIPU_API_KEY` | 智谱 CogView | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_BAIDU_API_KEY` | 百度文心一格 | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_BAIDU_SECRET_KEY` | 百度文心一格 | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_DOUBAO_API_KEY` | 豆包 | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_TENCENT_SECRET_ID` | 腾讯混元 | 图像 |
| `AGENTFLOW_MULTIMODAL_IMAGE_TENCENT_SECRET_KEY` | 腾讯混元 | 图像 |
| `AGENTFLOW_MULTIMODAL_VIDEO_GOOGLE_API_KEY` | Google Veo | 视频 |
| `AGENTFLOW_MULTIMODAL_VIDEO_RUNWAY_API_KEY` | Runway | 视频 |
| `AGENTFLOW_MULTIMODAL_VIDEO_VEO_API_KEY` | Google Veo | 视频 |
| `AGENTFLOW_MULTIMODAL_VIDEO_SORA_API_KEY` | OpenAI Sora | 视频 |
| `AGENTFLOW_MULTIMODAL_VIDEO_KLING_API_KEY` | 可灵 | 视频 |
| `AGENTFLOW_MULTIMODAL_VIDEO_LUMA_API_KEY` | Luma | 视频 |
| `AGENTFLOW_MULTIMODAL_VIDEO_MINIMAX_API_KEY` | MiniMax | 视频 |
| `AGENTFLOW_MULTIMODAL_VIDEO_SEEDANCE_API_KEY` | Seedance | 视频 |

#### 工具提供商密钥

启用外部工具集成时，需配置对应提供商的 API Key：

| 环境变量键 | 提供商 | 说明 |
|-----------|--------|------|
| `AGENTFLOW_TOOLS_TAVILY_API_KEY` | Tavily | 网络搜索（推荐） |
| `AGENTFLOW_TOOLS_JINA_API_KEY` | Jina Reader | 网页抓取（可选，免费可用） |
| `AGENTFLOW_TOOLS_FIRECRAWL_API_KEY` | Firecrawl | 搜索+抓取 |

#### 完整 Secret 示例

```bash
# 完整配置示例
kubectl create secret generic agentflow-secrets \
  --namespace agentflow \
  --from-literal=AGENTFLOW_LLM_API_KEY=sk-xxx \
  --from-literal=AGENTFLOW_SERVER_JWT_SECRET=$(openssl rand -base64 32) \
  --from-literal=AGENTFLOW_DATABASE_PASSWORD=db_password \
  --from-literal=AGENTFLOW_REDIS_PASSWORD=redis_password \
  --from-literal=AGENTFLOW_MONGODB_PASSWORD=mongo_password \
  --from-literal=AGENTFLOW_MULTIMODAL_IMAGE_OPENAI_API_KEY=sk-xxx \
  --from-literal=AGENTFLOW_MULTIMODAL_VIDEO_RUNWAY_API_KEY=xxx \
  --from-literal=AGENTFLOW_TOOLS_TAVILY_API_KEY=tvly-xxx
```

### 4. 安装 AgentFlow

```bash
helm install agentflow ./deployments/helm/agentflow \
  --namespace agentflow \
  --create-namespace \
  -f ./deployments/helm/agentflow/values-production.yaml \
  --set image.repository=your-registry/agentflow \
  --set image.tag=v1.0.0 \
  --set secrets.existingSecret=agentflow-secrets
```

### 5. 验证部署

```bash
# 检查 Pod 状态
kubectl get pods -n agentflow

# 检查服务
kubectl get svc -n agentflow

# 查看日志
kubectl logs -n agentflow -l app.kubernetes.io/name=agentflow -f
```

### 6. 访问服务

```bash
# 端口转发（开发测试）
kubectl port-forward -n agentflow svc/agentflow 8080:8080

# 健康检查
curl http://localhost:8080/health
```

#### 健康检查端点说明

AgentFlow 提供两个健康检查端点，用途不同：

| 端点 | 用途 | 检查内容 |
|------|------|---------|
| `/health` | 存活探针（liveness） | 进程存活状态，始终返回 200 |
| `/ready` | 就绪探针（readiness） | 依赖连接状态（数据库、Redis 等） |

**Kubernetes 探针配置**（Helm Chart 默认配置）：

| 探针类型 | 端点 | 初始延迟 | 检查间隔 | 超时 | 失败阈值 |
|---------|------|---------|---------|------|---------|
| `livenessProbe` | `/health` | 15s | 30s | 5s | 3 |
| `readinessProbe` | `/ready` | 10s | 15s | 5s | 3 |
| `startupProbe` | `/health` | - | 10s | 5s | 18（最多 3 分钟） |

**`/ready` 检查的依赖项**：

- 数据库连接（PostgreSQL/MongoDB，如配置）
- Redis 连接（如配置）
- 向量存储连接（Qdrant/Weaviate/Milvus，如配置）

当任一依赖不可用时，`/ready` 返回 503，Pod 将从 Service 端点中移除，但不触发重启。

**自定义探针配置**：

```yaml
# values.yaml
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 15
  periodSeconds: 30
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /ready
    port: http
  initialDelaySeconds: 10
  periodSeconds: 15
  timeoutSeconds: 5
  failureThreshold: 3
```

## Helm Chart 配置

### 基本配置

```bash
helm install agentflow ./deployments/helm/agentflow \
  --namespace agentflow \
  --set replicaCount=3 \
  --set image.tag=v1.0.0 \
  --set agent.model=gpt-4-turbo \
  --set secrets.existingSecret=agentflow-secrets
```

### 使用 values 文件

```yaml
# my-values.yaml
replicaCount: 3

image:
  repository: your-registry/agentflow
  tag: v1.0.0

agent:
  model: "gpt-4-turbo"
  maxIterations: 20
  temperature: "0.5"

log:
  level: "debug"

resources:
  limits:
    cpu: 2000m
    memory: 2Gi
  requests:
    cpu: 500m
    memory: 512Mi

autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

metrics:
  service:
    enabled: true

serviceMonitor:
  enabled: true

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: agentflow.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: agentflow-tls
      hosts:
        - agentflow.example.com
```

```bash
helm install agentflow ./deployments/helm/agentflow \
  --namespace agentflow \
  -f my-values.yaml
```

### 配置选项参考

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `replicaCount` | Pod 副本数 | 1 |
| `image.repository` | 镜像仓库 | agentflow |
| `image.tag` | 镜像标签 | Chart appVersion |
| `server.environment` | 运行环境（development/test/production） | `production` |
| `server.allowNoAuth` | 是否允许无认证启动 | `false` |
| `server.apiKeys` | 通过 YAML 配置的 HTTP API keys | `[]` |
| `server.metricsBindAddress` | Metrics 端口监听地址 | `127.0.0.1` |
| `server.enablePprof` | 启用 pprof 诊断端点 | `false` |
| `agent.model` | 默认模型 | gpt-4 |
| `agent.maxIterations` | 最大迭代次数 | 10 |
| `multimodal.enabled` | 启用多模态 API 路由 | `false` |
| `multimodal.defaultImageProvider` | 默认图像提供商 | - |
| `multimodal.defaultVideoProvider` | 默认视频提供商 | - |
| `metrics.service.enabled` | 是否创建 metrics Service | false |
| `serviceMonitor.enabled` | 是否创建 ServiceMonitor | false |
| `autoscaling.enabled` | 启用 HPA | false |
| `ingress.enabled` | 启用 Ingress | false |

#### 关键配置说明

##### `server.allowNoAuth`

控制是否允许在无认证配置（无 API keys、无 JWT）时启动服务。

- **默认值**：`false`
- **生产环境**：**强制为 false**，当 `server.environment=production` 时，若配置为 `true` 会在启动阶段直接报错拒绝启动
- **开发/测试环境**：可设为 `true` 便于快速验证，但绝不建议在生产环境使用

```yaml
# 仅允许在 development/test 环境开启
server:
  environment: development
  allowNoAuth: true  # 仅开发环境可用
```

##### `server.metricsBindAddress`

Metrics 端口（默认 9091）的监听地址。

- **默认值**：`127.0.0.1`（仅允许本地访问）
- **外部抓取**：若 Prometheus 需从 Pod 外部抓取指标，需设为 `0.0.0.0`

```yaml
server:
  metricsPort: 9091
  metricsBindAddress: "0.0.0.0"  # 允许外部抓取
```

##### `server.enablePprof`

是否在 Metrics 端口暴露 pprof 诊断端点。

- **默认值**：`false`
- **生产环境**：建议关闭，避免暴露性能分析数据
- **调试场景**：临时开启用于性能诊断

```yaml
server:
  enablePprof: true  # 临时开启用于调试
```

##### 多模态配置

启用图像/视频生成能力需要配置对应提供商的 API Key。详见下方 Secret 配置章节。

```yaml
multimodal:
  enabled: true
  defaultImageProvider: openai
  defaultVideoProvider: runway
  referenceStoreBackend: redis
  referenceMaxSizeBytes: 8388608  # 8MB
  referenceTTL: 2h
```

## 高可用部署

### 多副本配置

```yaml
# ha-values.yaml
replicaCount: 3

podDisruptionBudget:
  enabled: true
  minAvailable: 2

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchLabels:
              app.kubernetes.io/name: agentflow
          topologyKey: kubernetes.io/hostname

topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: ScheduleAnyway
    labelSelector:
      matchLabels:
        app.kubernetes.io/name: agentflow
```

### 自动扩缩容

```yaml
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 20
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 80
```

### 外部依赖（生产推荐）

```yaml
redis:
  addr: "redis.example.com:6379"

database:
  host: "postgres.example.com"
  port: "5432"
  user: "agentflow"
  name: "agentflow"
  sslMode: "require"

mongodb:
  host: "mongodb.example.com"
  port: "27017"
  user: "agentflow"
  database: "agentflow"

qdrant:
  host: "qdrant.example.com"
  port: "6334"

secrets:
  existingSecret: agentflow-secrets
```

`agentflow-secrets` 环境变量键列表请参考上方「Secret 环境变量键完整列表」章节。

说明：

- chart 当前通过挂载 `/app/config/config.yaml` 注入非敏感配置，因此 `server.api_keys` 走 values / YAML；
- 如果你不希望 API key 出现在 ConfigMap 中，生产环境优先改用 JWT，并通过 Secret 提供 `AGENTFLOW_SERVER_JWT_SECRET` 或 `AGENTFLOW_SERVER_JWT_PUBLIC_KEY`；
- `server.apiKeys` 仍可用于内网、短期测试或配合加密 values 文件的场景；
- 多模态提供商密钥和工具提供商密钥按需配置，未配置的提供商相关 API 将不可用。

## 安全配置

### 认证与授权

#### `allow_no_auth` 强制拒绝说明

AgentFlow 在启动时会对 `server.allow_no_auth` 配置进行安全校验：

| 环境 | `allow_no_auth=true` | `allow_no_auth=false` |
|------|---------------------|----------------------|
| `development` | 允许（用于本地调试） | 正常启动，需配置认证 |
| `test` | 允许（用于自动化测试） | 正常启动，需配置认证 |
| `production` | **直接报错拒绝启动** | 正常启动，需配置认证 |

**生产环境强制要求**：

- `server.environment=production` 时，`allow_no_auth=true` 会触发启动错误
- 必须配置至少一种认证方式：
  - `server.api_keys`（ConfigMap/YAML 配置，适合简单场景）
  - `AGENTFLOW_SERVER_JWT_SECRET`（HMAC 签名，推荐）
  - `AGENTFLOW_SERVER_JWT_PUBLIC_KEY`（RSA 公钥验证，适合企业集成）

```yaml
# 生产环境配置示例
server:
  environment: production
  allowNoAuth: false  # 强制要求，即使不配置也会默认 false
  jwt:
    issuer: "agentflow"
    audience: "api"
    expiration: "1h"
```

#### 配置热重载安全说明

AgentFlow 支持部分配置的热重载（无需重启 Pod），但有以下安全注意事项：

| 配置项 | 热重载支持 | 安全建议 |
|--------|-----------|---------|
| 工具注册绑定 | 是 | 仅重载工具别名和参数模板，不涉及认证变更 |
| Web 搜索提供商优先级 | 是 | 从数据库读取，需确保数据库访问权限安全 |
| API Keys / JWT 密钥 | 否 | 需重启 Pod 生效，建议通过 Secret 管理并滚动更新 |

**最佳实践**：

1. 敏感配置（密钥、密码）统一通过 Kubernetes Secret 管理
2. 使用 External Secrets Operator 从外部密钥管理系统同步
3. 密钥轮换时，先更新 Secret，再执行 `kubectl rollout restart` 滚动更新 Pod

### Pod 安全上下文

```yaml
podSecurityContext:
  fsGroup: 1000
  runAsNonRoot: true

securityContext:
  capabilities:
    drop:
      - ALL
  readOnlyRootFilesystem: true
  runAsNonRoot: true
  runAsUser: 1000
  allowPrivilegeEscalation: false
```

### 网络策略

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: agentflow-network-policy
  namespace: agentflow
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: agentflow
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - protocol: TCP
          port: 8080
  egress:
    - to:
        - namespaceSelector: {}
      ports:
        - protocol: TCP
          port: 6379  # Redis
        - protocol: TCP
          port: 5432  # PostgreSQL
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - protocol: TCP
          port: 443  # LLM API
```

### Secret 管理

```bash
# 使用 External Secrets Operator
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: agentflow-secrets
  namespace: agentflow
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: agentflow-secrets
  data:
    - secretKey: AGENTFLOW_LLM_API_KEY
      remoteRef:
        key: agentflow/llm
        property: api-key
    - secretKey: AGENTFLOW_SERVER_JWT_SECRET
      remoteRef:
        key: agentflow/http
        property: jwt-secret
```

## 监控集成

### Prometheus ServiceMonitor

```yaml
metrics:
  service:
    enabled: true

serviceMonitor:
  enabled: true
  interval: 30s
  scrapeTimeout: 10s
  labels:
    release: prometheus
```

### Grafana 仪表盘

```bash
# 导入仪表盘
kubectl create configmap agentflow-dashboard \
  --namespace monitoring \
  --from-file=dashboard.json \
  --dry-run=client -o yaml | kubectl apply -f -
```

### 告警规则

```yaml
# prometheus-rules.yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: agentflow-alerts
  namespace: agentflow
spec:
  groups:
    - name: agentflow
      rules:
        - alert: AgentFlowHighErrorRate
          expr: |
            sum(rate(agentflow_requests_total{status="error"}[5m]))
            / sum(rate(agentflow_requests_total[5m])) > 0.05
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "AgentFlow error rate is high"
            description: "Error rate is {{ $value | humanizePercentage }}"

        - alert: AgentFlowPodNotReady
          expr: |
            kube_pod_status_ready{namespace="agentflow", condition="true"} == 0
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: "AgentFlow pod is not ready"
```

## 升级和回滚

### 升级

```bash
# 查看当前版本
helm list -n agentflow

# 升级到新版本
helm upgrade agentflow ./deployments/helm/agentflow \
  --namespace agentflow \
  -f ./deployments/helm/agentflow/values-production.yaml \
  --set image.tag=v1.1.0

# 查看升级历史
helm history agentflow -n agentflow
```

### 回滚

```bash
# 回滚到上一版本
helm rollback agentflow -n agentflow

# 回滚到指定版本
helm rollback agentflow 2 -n agentflow
```

### 蓝绿部署

```bash
# 部署新版本到新命名空间
kubectl create namespace agentflow-green
helm install agentflow-green ./deployments/helm/agentflow \
  --namespace agentflow-green \
  --create-namespace \
  -f ./deployments/helm/agentflow/values-production.yaml \
  --set image.tag=v1.1.0

# 验证新版本
kubectl port-forward -n agentflow-green svc/agentflow-green 8081:8080
curl http://localhost:8081/health

# 切换流量（更新 Ingress）
kubectl patch ingress agentflow -n agentflow \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/rules/0/http/paths/0/backend/service/name", "value":"agentflow-green"}]'
```

## 故障排除

### 常用诊断命令

```bash
# 查看 Pod 状态
kubectl get pods -n agentflow -o wide

# 查看 Pod 详情
kubectl describe pod -n agentflow <pod-name>

# 查看日志
kubectl logs -n agentflow <pod-name> --tail=100

# 进入容器
kubectl exec -it -n agentflow <pod-name> -- /bin/sh

# 查看事件
kubectl get events -n agentflow --sort-by='.lastTimestamp'
```

### 常见问题

#### 0. `helm install` 失败，提示 chart 不存在

```bash
ls deployments/helm/agentflow
helm lint ./deployments/helm/agentflow
```

如果 `deployments/helm/agentflow/` 缺少 `Chart.yaml` 或 `templates/`，说明当前工作区未同步到包含正式 Helm Chart 的版本。

#### 1. Pod 处于 Pending 状态

```bash
# 检查资源
kubectl describe pod -n agentflow <pod-name>

# 检查节点资源
kubectl top nodes
```

#### 2. Pod 处于 CrashLoopBackOff

```bash
# 查看日志
kubectl logs -n agentflow <pod-name> --previous

# 检查配置
kubectl get configmap -n agentflow -l app.kubernetes.io/name=agentflow -o yaml
```

#### 3. 无法连接服务

```bash
# 检查服务
kubectl get svc -n agentflow

# 检查端点
kubectl get endpoints -n agentflow

# 测试 DNS
kubectl run -it --rm debug --image=busybox --restart=Never -- nslookup agentflow.agentflow.svc.cluster.local
```

#### 4. Ingress 不工作

```bash
# 检查 Ingress
kubectl describe ingress -n agentflow agentflow

# 检查 Ingress Controller 日志
kubectl logs -n ingress-nginx -l app.kubernetes.io/name=ingress-nginx
```

## 下一步

- [生产环境最佳实践](./production.md)
- [监控和告警配置](./monitoring.md)
- [备份和恢复](./backup.md)

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
kubectl create secret generic agentflow-secrets \
  --namespace agentflow \
  --from-literal=llm-api-key=your_api_key_here
```

### 4. 安装 AgentFlow

```bash
helm install agentflow ./deployments/helm/agentflow \
  --namespace agentflow \
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

## Helm Chart 配置

### 基本配置

```bash
helm install agentflow ./deployments/helm/agentflow \
  --namespace agentflow \
  --set replicaCount=3 \
  --set image.tag=v1.0.0 \
  --set config.agent.model=gpt-4-turbo \
  --set secrets.existingSecret=agentflow-secrets
```

### 使用 values 文件

```yaml
# my-values.yaml
replicaCount: 3

image:
  repository: your-registry/agentflow
  tag: v1.0.0

config:
  agent:
    model: "gpt-4-turbo"
    maxIterations: 20
    temperature: 0.5

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
| `config.agent.model` | 默认模型 | gpt-4 |
| `config.agent.maxIterations` | 最大迭代次数 | 10 |
| `redis.enabled` | 启用内置 Redis | true |
| `postgresql.enabled` | 启用内置 PostgreSQL | false |
| `autoscaling.enabled` | 启用 HPA | false |
| `ingress.enabled` | 启用 Ingress | false |

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

### 外部数据库（生产推荐）

```yaml
redis:
  enabled: false
  external:
    host: "redis.example.com"
    port: 6379
    password: "your_redis_password"

postgresql:
  enabled: false
  external:
    host: "postgres.example.com"
    port: 5432
    user: "agentflow"
    password: "your_db_password"
    database: "agentflow"
    sslMode: "require"
```

## 安全配置

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
    - secretKey: llm-api-key
      remoteRef:
        key: agentflow/llm
        property: api-key
```

## 监控集成

### Prometheus ServiceMonitor

```yaml
metrics:
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
  -f my-values.yaml \
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
kubectl get configmap -n agentflow agentflow-config -o yaml
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

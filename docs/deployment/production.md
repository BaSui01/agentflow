# 🏭 生产环境最佳实践

本文档提供 AgentFlow 生产环境部署的最佳实践和建议。

## 📋 目录

- [部署清单](#部署清单)
- [性能优化](#性能优化)
- [安全加固](#安全加固)
- [可靠性保障](#可靠性保障)
- [监控告警](#监控告警)
- [备份恢复](#备份恢复)
- [成本优化](#成本优化)

## 部署清单

### 上线前检查清单

- [ ] **基础设施**
  - [ ] 使用托管 Kubernetes 服务（EKS/GKE/AKS）
  - [ ] 配置多可用区部署
  - [ ] 设置适当的资源配额
  - [ ] 配置网络策略

- [ ] **安全**
  - [ ] API Key 存储在 Secret Manager
  - [ ] 启用 TLS/HTTPS
  - [ ] 配置 RBAC 权限
  - [ ] 启用审计日志
  - [ ] 扫描容器镜像漏洞

- [ ] **可靠性**
  - [ ] 配置 HPA 自动扩缩容
  - [ ] 设置 PDB（Pod 中断预算）
  - [ ] 配置健康检查探针
  - [ ] 设置资源限制

- [ ] **监控**
  - [ ] 配置 Prometheus 指标收集
  - [ ] 设置 Grafana 仪表盘
  - [ ] 配置告警规则
  - [ ] 启用分布式追踪

- [ ] **备份**
  - [ ] 配置数据库自动备份
  - [ ] 测试恢复流程
  - [ ] 文档化灾难恢复计划

## 性能优化

### 资源配置建议

```yaml
# 生产环境资源配置
resources:
  requests:
    cpu: 500m
    memory: 512Mi
  limits:
    cpu: 2000m
    memory: 2Gi
```

### 连接池优化

```yaml
# config.yaml
redis:
  pool_size: 20
  min_idle_conns: 5

database:
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime: "5m"
```

### LLM 调用优化

```yaml
agent:
  # 合理设置迭代次数，避免无限循环
  max_iterations: 15

  # 设置合理的超时时间
  timeout: "3m"

  # 启用流式输出减少首字节时间
  stream_enabled: true

llm:
  # 设置重试策略
  max_retries: 3

  # 请求超时
  timeout: "2m"
```

### 缓存策略

```yaml
# 启用 Redis 缓存
redis:
  enabled: true

# 配置记忆管理
agent:
  memory:
    enabled: true
    type: "buffer"
    max_messages: 50  # 限制上下文长度
    token_limit: 4000
```

## 安全加固

### Secret 管理

```bash
# 使用 AWS Secrets Manager
aws secretsmanager create-secret \
  --name agentflow/llm-api-key \
  --secret-string "your_api_key"

# 使用 External Secrets Operator 同步
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: agentflow-secrets
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: aws-secrets-manager
    kind: ClusterSecretStore
  target:
    name: agentflow-secrets
  data:
    - secretKey: llm-api-key
      remoteRef:
        key: agentflow/llm-api-key
```

### 网络安全

```yaml
# 网络策略 - 最小权限原则
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: agentflow-strict
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
              network-policy: allow-agentflow
      ports:
        - port: 8080
  egress:
    # 只允许访问必要的服务
    - to:
        - podSelector:
            matchLabels:
              app: redis
      ports:
        - port: 6379
    - to:
        - podSelector:
            matchLabels:
              app: postgresql
      ports:
        - port: 5432
    # LLM API (HTTPS)
    - to:
        - ipBlock:
            cidr: 0.0.0.0/0
      ports:
        - port: 443
          protocol: TCP
```

### TLS 配置

```yaml
# Ingress TLS
ingress:
  enabled: true
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
  tls:
    - secretName: agentflow-tls
      hosts:
        - agentflow.example.com
```

### Pod 安全标准

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

## 可靠性保障

### 高可用配置

```yaml
# 生产环境 HA 配置
replicaCount: 3

podDisruptionBudget:
  enabled: true
  minAvailable: 2

affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchLabels:
            app.kubernetes.io/name: agentflow
        topologyKey: kubernetes.io/hostname

topologySpreadConstraints:
  - maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
    whenUnsatisfiable: DoNotSchedule
    labelSelector:
      matchLabels:
        app.kubernetes.io/name: agentflow
```

### 健康检查

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 15
  periodSeconds: 20
  timeoutSeconds: 5
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
  timeoutSeconds: 3
  failureThreshold: 3

startupProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10
  timeoutSeconds: 5
  failureThreshold: 30
```

### 优雅关闭

```yaml
# 确保有足够时间完成请求
terminationGracePeriodSeconds: 60

# 配置 preStop hook
lifecycle:
  preStop:
    exec:
      command:
        - /bin/sh
        - -c
        - sleep 10
```

### 自动扩缩容

```yaml
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 20
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Percent
          value: 10
          periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
        - type: Percent
          value: 100
          periodSeconds: 15
        - type: Pods
          value: 4
          periodSeconds: 15
      selectPolicy: Max
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

## 监控告警

### 关键指标

| 指标 | 告警阈值 | 说明 |
|------|----------|------|
| 错误率 | > 1% | 5分钟内错误请求占比 |
| P99 延迟 | > 10s | 99分位响应时间 |
| CPU 使用率 | > 80% | 持续5分钟 |
| 内存使用率 | > 85% | 持续5分钟 |
| Pod 重启 | > 3次/小时 | 异常重启 |

### Prometheus 告警规则

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: agentflow-production-alerts
spec:
  groups:
    - name: agentflow.critical
      rules:
        - alert: AgentFlowDown
          expr: up{job="agentflow"} == 0
          for: 1m
          labels:
            severity: critical
          annotations:
            summary: "AgentFlow is down"
            runbook_url: "https://wiki.example.com/agentflow/runbook#down"

        - alert: AgentFlowHighErrorRate
          expr: |
            sum(rate(agentflow_requests_total{status="error"}[5m]))
            / sum(rate(agentflow_requests_total[5m])) > 0.01
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: "AgentFlow error rate > 1%"

        - alert: AgentFlowHighLatency
          expr: |
            histogram_quantile(0.99,
              sum(rate(agentflow_request_duration_seconds_bucket[5m])) by (le)
            ) > 10
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: "AgentFlow P99 latency > 10s"

    - name: agentflow.capacity
      rules:
        - alert: AgentFlowHighCPU
          expr: |
            sum(rate(container_cpu_usage_seconds_total{
              namespace="agentflow",
              container="agentflow"
            }[5m])) by (pod)
            / sum(kube_pod_container_resource_limits{
              namespace="agentflow",
              container="agentflow",
              resource="cpu"
            }) by (pod) > 0.8
          for: 5m
          labels:
            severity: warning

        - alert: AgentFlowHighMemory
          expr: |
            sum(container_memory_working_set_bytes{
              namespace="agentflow",
              container="agentflow"
            }) by (pod)
            / sum(kube_pod_container_resource_limits{
              namespace="agentflow",
              container="agentflow",
              resource="memory"
            }) by (pod) > 0.85
          for: 5m
          labels:
            severity: warning
```

### 日志聚合

```yaml
# 结构化日志配置
log:
  level: "info"
  format: "json"
  output_paths:
    - "stdout"

# Fluentd/Fluent Bit 收集配置
# 发送到 Elasticsearch/Loki
```

## 备份恢复

### 数据库备份

```bash
# PostgreSQL 自动备份 CronJob
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-backup
spec:
  schedule: "0 2 * * *"  # 每天凌晨2点
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: backup
              image: postgres:16
              command:
                - /bin/sh
                - -c
                - |
                  pg_dump -h $PGHOST -U $PGUSER $PGDATABASE | \
                  gzip > /backup/agentflow-$(date +%Y%m%d).sql.gz
              env:
                - name: PGPASSWORD
                  valueFrom:
                    secretKeyRef:
                      name: postgres-credentials
                      key: password
              volumeMounts:
                - name: backup
                  mountPath: /backup
          volumes:
            - name: backup
              persistentVolumeClaim:
                claimName: backup-pvc
          restartPolicy: OnFailure
```

### 灾难恢复计划

1. **RTO（恢复时间目标）**: 30分钟
2. **RPO（恢复点目标）**: 1小时

#### 恢复步骤

```bash
# 1. 确认故障范围
kubectl get pods -n agentflow
kubectl get events -n agentflow

# 2. 如果是配置问题，回滚到上一版本
helm rollback agentflow -n agentflow

# 3. 如果是数据问题，从备份恢复
kubectl exec -it postgres-0 -- psql -U agentflow -c "DROP DATABASE agentflow;"
kubectl exec -it postgres-0 -- psql -U agentflow -c "CREATE DATABASE agentflow;"
gunzip -c backup.sql.gz | kubectl exec -i postgres-0 -- psql -U agentflow agentflow

# 4. 验证恢复
curl http://agentflow.example.com/health
```

## 成本优化

### 资源优化

```yaml
# 使用 VPA 自动调整资源
apiVersion: autoscaling.k8s.io/v1
kind: VerticalPodAutoscaler
metadata:
  name: agentflow-vpa
spec:
  targetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: agentflow
  updatePolicy:
    updateMode: "Auto"
  resourcePolicy:
    containerPolicies:
      - containerName: agentflow
        minAllowed:
          cpu: 100m
          memory: 128Mi
        maxAllowed:
          cpu: 4
          memory: 4Gi
```

### Spot/Preemptible 实例

```yaml
# 使用 Spot 实例降低成本（非关键工作负载）
affinity:
  nodeAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        preference:
          matchExpressions:
            - key: node.kubernetes.io/lifecycle
              operator: In
              values:
                - spot

tolerations:
  - key: "kubernetes.azure.com/scalesetpriority"
    operator: "Equal"
    value: "spot"
    effect: "NoSchedule"
```

### LLM 成本控制

```yaml
agent:
  # 限制 Token 使用
  max_tokens: 2048

  # 使用更经济的模型处理简单任务
  model: "gpt-3.5-turbo"

  # 限制迭代次数
  max_iterations: 10

  # 启用缓存减少重复调用
  memory:
    enabled: true
```

## 总结

生产环境部署的核心原则：

1. **安全第一** - 最小权限、加密传输、Secret 管理
2. **高可用** - 多副本、多可用区、自动扩缩容
3. **可观测** - 指标、日志、追踪、告警
4. **可恢复** - 定期备份、灾难恢复演练
5. **成本效益** - 资源优化、按需扩缩

定期审查和更新这些配置，确保系统始终处于最佳状态！

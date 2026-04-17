# 监控和告警配置

> AgentFlow 生产环境监控指南

## 概述

AgentFlow 内置 Prometheus 指标导出和 OpenTelemetry 分布式追踪支持，可与主流监控平台集成。

## Prometheus 指标

### 启用指标导出

```yaml
server:
  metrics_port: 9091
  metrics_bind_address: "127.0.0.1"
  enable_pprof: false
```

说明：
- `/metrics` 运行在独立 metrics 端口，默认仅绑定 loopback
- 若 Prometheus 需要跨容器或跨节点抓取，请显式将 `metrics_bind_address` 设为 `0.0.0.0`
- `pprof` 默认关闭，只建议在受控排障窗口中短时开启
- Helm Chart 场景下，开启 `metrics.service.enabled` 或 `serviceMonitor.enabled` 会自动把 `metrics_bind_address` 切到 `0.0.0.0`

### 核心指标

| 指标名称 | 类型 | 说明 |
|---------|------|------|
| `agentflow_llm_requests_total` | Counter | LLM 请求总数 |
| `agentflow_llm_request_duration_seconds` | Histogram | LLM 请求延迟分布 |
| `agentflow_llm_tokens_total` | Counter | Token 使用总量 |
| `agentflow_llm_errors_total` | Counter | LLM 错误总数 |
| `agentflow_provider_health` | Gauge | Provider 健康状态 |

## OpenTelemetry 追踪

```yaml
telemetry:
  enabled: true
  otlp_endpoint: "localhost:4317"
  otlp_insecure: false
  service_name: "agentflow"
  sample_rate: 0.1
```

## Helm 集成

### ServiceMonitor

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

对应资源由 `deployments/helm/agentflow/templates/servicemonitor.yaml` 生成，抓取目标为独立 metrics Service 的 `metrics` 端口。

### 监控暴露边界

- 默认只创建业务 HTTP Service，不额外暴露 metrics Service。
- 只有在 `metrics.service.enabled=true` 或 `serviceMonitor.enabled=true` 时才会创建 `*-metrics` Service。
- 即使暴露了 metrics Service，`pprof` 仍默认关闭；开启 `server.enablePProf=true` 前应确认只在内网或受控代理后访问。

## Grafana 仪表盘

推荐使用 Grafana 可视化监控数据，可导入项目提供的仪表盘模板。

## 告警规则

### 示例：Provider 错误率告警

```yaml
groups:
  - name: agentflow
    rules:
      - alert: HighProviderErrorRate
        expr: rate(agentflow_llm_errors_total[5m]) / rate(agentflow_llm_requests_total[5m]) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Provider 错误率过高"
```

## 相关文档

- [Kubernetes 部署](./kubernetes.md)
- [Docker 部署](./docker.md)
- [备份和恢复](./backup.md)

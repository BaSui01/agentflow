# 监控和告警配置

> AgentFlow 生产环境监控指南

## 概述

AgentFlow 内置 Prometheus 指标导出和 OpenTelemetry 分布式追踪支持，可与主流监控平台集成。

## Prometheus 指标

### 启用指标导出

```yaml
observability:
  metrics:
    enabled: true
    port: 9090
    path: /metrics
```

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
observability:
  tracing:
    enabled: true
    exporter: otlp
    endpoint: localhost:4317
```

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

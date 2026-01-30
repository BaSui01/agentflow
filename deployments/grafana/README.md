# AgentFlow Grafana Dashboards

This directory contains pre-built Grafana dashboard templates for monitoring AgentFlow applications.

## Available Dashboards

| Dashboard | File | Description |
|-----------|------|-------------|
| **AgentFlow Overview** | `agentflow-dashboard.json` | High-level overview with key metrics across all components |
| **LLM Metrics** | `agentflow-llm-dashboard.json` | Detailed LLM provider metrics, token usage, and costs |
| **System Resources** | `agentflow-system-dashboard.json` | Go runtime, CPU, memory, and goroutine metrics |
| **Workflow Metrics** | `agentflow-workflow-dashboard.json` | DAG workflow execution and node-level metrics |

## Prerequisites

- Grafana 9.0+ (recommended: 10.x)
- Prometheus data source configured
- AgentFlow application exposing metrics on `/metrics` endpoint

## Importing Dashboards

### Method 1: Grafana UI Import

1. Open Grafana and navigate to **Dashboards** > **Import**
2. Click **Upload JSON file** and select the desired dashboard JSON file
3. Select your Prometheus data source
4. Click **Import**

### Method 2: Grafana Provisioning

Add the following to your Grafana provisioning configuration:

```yaml
# /etc/grafana/provisioning/dashboards/agentflow.yaml
apiVersion: 1

providers:
  - name: 'AgentFlow'
    orgId: 1
    folder: 'AgentFlow'
    folderUid: 'agentflow'
    type: file
    disableDeletion: false
    updateIntervalSeconds: 30
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards/agentflow
```

Then copy the JSON files to `/var/lib/grafana/dashboards/agentflow/`.

### Method 3: Kubernetes ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agentflow-grafana-dashboards
  labels:
    grafana_dashboard: "1"
data:
  agentflow-dashboard.json: |
    <contents of agentflow-dashboard.json>
```

If using the Grafana Helm chart with sidecar enabled, dashboards will be automatically loaded.

## Prometheus Configuration

Ensure your Prometheus is configured to scrape AgentFlow metrics:

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'agentflow'
    static_configs:
      - targets: ['agentflow:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```

For Kubernetes deployments with ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: agentflow
  labels:
    release: prometheus
spec:
  selector:
    matchLabels:
      app: agentflow
  endpoints:
    - port: http
      path: /metrics
      interval: 15s
```

## Metrics Reference

### LLM Provider Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `llm_provider_healthy` | Gauge | `provider_id` | Provider health status (1=healthy, 0=unhealthy) |
| `llm_provider_health_check_latency_ms` | Histogram | `provider_id` | Health check latency in milliseconds |
| `llm_provider_health_check_failures_total` | Counter | `provider_id` | Total health check failures |
| `agentflow_llm_requests_total` | Counter | `provider`, `model`, `status` | Total LLM API requests |
| `agentflow_llm_request_duration_seconds` | Histogram | `provider`, `model` | LLM request duration |
| `agentflow_llm_tokens_total` | Counter | `provider`, `model`, `type` | Total tokens used (input/output) |
| `agentflow_llm_cost_total` | Counter | `provider`, `model` | Total API cost in USD |

### Agent Execution Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `agentflow_agent_executions_total` | Counter | `agent_type`, `status` | Total agent executions |
| `agentflow_agent_execution_duration_seconds` | Histogram | `agent_type` | Agent execution duration |

### Workflow Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `agentflow_workflow_executions_total` | Counter | `workflow_name`, `status` | Total workflow executions |
| `agentflow_workflow_duration_seconds` | Histogram | `workflow_name` | Workflow execution duration |
| `agentflow_workflow_active_executions` | Gauge | - | Currently active workflow executions |
| `agentflow_workflow_nodes_executed` | Histogram | `workflow_name` | Nodes executed per workflow |
| `agentflow_dag_node_executions_total` | Counter | `node_type`, `status` | Total DAG node executions |
| `agentflow_dag_node_duration_seconds` | Histogram | `node_type` | DAG node execution duration |
| `agentflow_checkpoint_saves_total` | Counter | `status` | Checkpoint save operations |
| `agentflow_checkpoint_loads_total` | Counter | `status` | Checkpoint load operations |
| `agentflow_workflow_recoveries_total` | Counter | `status` | Workflow recovery attempts |
| `agentflow_workflow_retries_total` | Counter | - | Workflow retry attempts |

### Go Runtime Metrics (Standard)

| Metric | Type | Description |
|--------|------|-------------|
| `go_goroutines` | Gauge | Number of goroutines |
| `go_threads` | Gauge | Number of OS threads |
| `go_gc_duration_seconds` | Summary | GC pause duration |
| `go_memstats_alloc_bytes` | Gauge | Bytes allocated and in use |
| `go_memstats_heap_inuse_bytes` | Gauge | Heap bytes in use |
| `process_cpu_seconds_total` | Counter | Total CPU time |
| `process_resident_memory_bytes` | Gauge | Resident memory size |
| `process_open_fds` | Gauge | Open file descriptors |

## Dashboard Variables

All dashboards support the following template variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `datasource` | Prometheus data source | Auto-detected |
| `provider` | LLM provider filter | All |
| `model` | LLM model filter | All |
| `job` | Prometheus job name | `.*agentflow.*` |
| `workflow` | Workflow name filter | All |

## Alerting

Example Prometheus alerting rules for AgentFlow:

```yaml
groups:
  - name: agentflow
    rules:
      - alert: LLMProviderUnhealthy
        expr: llm_provider_healthy == 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "LLM provider {{ $labels.provider_id }} is unhealthy"

      - alert: HighLLMErrorRate
        expr: |
          sum(rate(agentflow_llm_requests_total{status="error"}[5m]))
          / sum(rate(agentflow_llm_requests_total[5m])) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "LLM error rate is above 5%"

      - alert: HighWorkflowErrorRate
        expr: |
          sum(rate(agentflow_workflow_executions_total{status="error"}[5m]))
          / sum(rate(agentflow_workflow_executions_total[5m])) > 0.1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Workflow error rate is above 10%"

      - alert: HighGoroutineCount
        expr: go_goroutines > 5000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Goroutine count is unusually high"

      - alert: HighMemoryUsage
        expr: process_resident_memory_bytes > 2147483648
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Memory usage exceeds 2GB"
```

## Customization

### Adding Custom Panels

1. Import the dashboard
2. Click **Add** > **Visualization**
3. Configure your custom panel
4. Save the dashboard

### Modifying Thresholds

Each panel has configurable thresholds. To modify:

1. Edit the panel
2. Go to **Field** > **Thresholds**
3. Adjust the values and colors
4. Save

## Troubleshooting

### No Data Displayed

1. Verify Prometheus is scraping AgentFlow metrics:
   ```bash
   curl http://agentflow:8080/metrics
   ```

2. Check Prometheus targets:
   - Navigate to Prometheus UI > Status > Targets
   - Ensure AgentFlow target is UP

3. Verify metric names match your application's exported metrics

### Dashboard Variables Not Populating

1. Ensure the Prometheus data source is correctly configured
2. Check that metrics with the expected labels exist
3. Refresh the dashboard variables manually

## Support

For issues or feature requests, please open an issue in the AgentFlow repository.

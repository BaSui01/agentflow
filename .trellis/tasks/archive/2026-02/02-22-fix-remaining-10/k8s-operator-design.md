# K8s Operator Design Document

> Design proposal for AgentFlow Kubernetes Operator

## Overview

The AgentFlow K8s Operator manages Agent lifecycle on Kubernetes using Custom Resource Definitions (CRDs). It automates deployment, scaling, health monitoring, and configuration updates for AgentFlow agents.

## CRD Design

### AgentFlow CRD (`agentflow.io/v1alpha1`)

```yaml
apiVersion: agentflow.io/v1alpha1
kind: Agent
metadata:
  name: research-agent
  namespace: default
spec:
  # Agent configuration (maps to agent.Config)
  type: assistant
  model: gpt-4o
  provider: openai
  maxTokens: 4096
  temperature: 0.7

  # Runtime configuration
  replicas: 2
  resources:
    requests:
      memory: "256Mi"
      cpu: "250m"
    limits:
      memory: "512Mi"
      cpu: "500m"

  # Provider credentials (Secret reference)
  providerSecretRef:
    name: openai-credentials
    key: api-key

  # Feature flags
  features:
    reflection: true
    toolSelection: true
    enhancedMemory: false

  # Health check
  healthCheck:
    intervalSeconds: 30
    timeoutSeconds: 10

status:
  phase: Running  # Pending | Running | Failed | Stopped
  replicas: 2
  readyReplicas: 2
  lastHealthCheck: "2026-02-22T10:00:00Z"
  conditions:
    - type: Available
      status: "True"
      lastTransitionTime: "2026-02-22T09:00:00Z"
```

### AgentWorkflow CRD

```yaml
apiVersion: agentflow.io/v1alpha1
kind: AgentWorkflow
metadata:
  name: research-pipeline
spec:
  steps:
    - name: research
      agentRef: research-agent
    - name: summarize
      agentRef: summarizer-agent
      dependsOn: [research]
  schedule: "0 */6 * * *"  # Optional cron schedule
```

## Controller Architecture

```
cmd/operator/
  main.go                    # Operator entrypoint
internal/operator/
  controller/
    agent_controller.go      # Reconcile Agent CRD
    workflow_controller.go   # Reconcile AgentWorkflow CRD
  webhook/
    agent_webhook.go         # Admission webhook for validation
  config/
    crd/                     # CRD YAML manifests
    rbac/                    # RBAC rules
```

### Reconciliation Loop (Agent Controller)

```
Observe: Watch Agent CR changes
  |
  v
Diff: Compare desired state (spec) vs actual state (Deployment + Service)
  |
  v
Act:
  - Create/Update Deployment (agent container + sidecar)
  - Create/Update Service (for A2A/MCP endpoints)
  - Create/Update ConfigMap (agent config)
  - Update Status (phase, conditions)
```

### Key Design Decisions

1. Each Agent CR maps to one Deployment + one Service
2. Provider credentials are always referenced via Secrets (never inline)
3. Agent config is stored in a ConfigMap, mounted as a volume
4. Health checks use the existing `HealthCheck` method on `llm.Provider`
5. The operator uses `controller-runtime` (kubebuilder framework)

## Relationship with Existing Code

| Existing Package | Operator Usage |
|-----------------|----------------|
| `agent/k8s/operator.go` | Base scaffolding — extend with CRD reconciler |
| `agent/deployment/` | Deployment strategy logic (reuse for rollout) |
| `config/` | Agent config serialization (YAML) |
| `llm/factory/` | Provider creation from CRD spec |
| `agent/discovery/` | Service discovery for multi-agent |

## Implementation Roadmap

### Phase 1: Foundation (1-2 weeks)
- Define CRD schemas (`Agent`, `AgentWorkflow`)
- Scaffold controller with kubebuilder
- Implement basic Agent reconciler (create/delete Deployment)

### Phase 2: Lifecycle (1-2 weeks)
- Health check integration
- Status reporting and conditions
- ConfigMap-based hot reload

### Phase 3: Multi-Agent (2-3 weeks)
- AgentWorkflow controller
- Service mesh integration for A2A
- Auto-scaling based on queue depth

### Phase 4: Production Hardening (1-2 weeks)
- Admission webhooks for validation
- Metrics export (Prometheus)
- Leader election for HA operator

## Dependencies

- `sigs.k8s.io/controller-runtime` v0.17+
- `k8s.io/api` v0.29+
- `k8s.io/apimachinery` v0.29+

## Out of Scope (This Phase)

- Multi-cluster support
- Custom scheduler for agent placement
- GPU resource management
- Operator Lifecycle Manager (OLM) packaging

package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// AgentCRD代表一个代理的自定义资源定义.
type AgentCRD struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   ObjectMeta     `json:"metadata"`
	Spec       AgentSpec      `json:"spec"`
	Status     AgentCRDStatus `json:"status,omitempty"`
}

// ObjectMeta包含标准Kubernetes对象元数据.
type ObjectMeta struct {
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	Labels          map[string]string `json:"labels,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
	UID             string            `json:"uid,omitempty"`
	ResourceVersion string            `json:"resourceVersion,omitempty"`
	Generation      int64             `json:"generation,omitempty"`
	CreationTime    time.Time         `json:"creationTimestamp,omitempty"`
}

// AgentSpec定义了Agent的理想状态.
type AgentSpec struct {
	AgentType    string            `json:"agentType"`
	Replicas     int32             `json:"replicas"`
	Model        ModelSpec         `json:"model"`
	Resources    ResourceSpec      `json:"resources"`
	Scaling      ScalingSpec       `json:"scaling,omitempty"`
	HealthCheck  HealthCheckSpec   `json:"healthCheck,omitempty"`
	Environment  map[string]string `json:"environment,omitempty"`
	ConfigMapRef string            `json:"configMapRef,omitempty"`
	SecretRef    string            `json:"secretRef,omitempty"`
}

// ModelSpec定义了LLM模型配置.
type ModelSpec struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"maxTokens,omitempty"`
}

// 资源Spec定义了所需资源.
type ResourceSpec struct {
	Requests ResourceQuantity `json:"requests,omitempty"`
	Limits   ResourceQuantity `json:"limits,omitempty"`
}

// 资源数量定义了CPU和内存数量.
type ResourceQuantity struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	GPU    string `json:"gpu,omitempty"`
}

// 缩放Spec 定义自动缩放配置 。
type ScalingSpec struct {
	Enabled        bool             `json:"enabled"`
	MinReplicas    int32            `json:"minReplicas"`
	MaxReplicas    int32            `json:"maxReplicas"`
	TargetMetrics  []TargetMetric   `json:"targetMetrics,omitempty"`
	ScaleDownDelay time.Duration    `json:"scaleDownDelay,omitempty"`
	ScaleUpDelay   time.Duration    `json:"scaleUpDelay,omitempty"`
	CooldownPeriod time.Duration    `json:"cooldownPeriod,omitempty"`
	CustomBehavior *ScalingBehavior `json:"behavior,omitempty"`
}

// 目标计量定义了衡量尺度目标。
type TargetMetric struct {
	Type               string `json:"type"` // cpu, memory, custom, requests_per_second, latency
	Name               string `json:"name,omitempty"`
	TargetValue        int64  `json:"targetValue"`
	TargetAverageValue int64  `json:"targetAverageValue,omitempty"`
}

// 缩放行为定义自定义缩放行为 。
type ScalingBehavior struct {
	ScaleUp   *ScalingRules `json:"scaleUp,omitempty"`
	ScaleDown *ScalingRules `json:"scaleDown,omitempty"`
}

// 缩放规则定义了缩放规则.
type ScalingRules struct {
	StabilizationWindowSeconds int32           `json:"stabilizationWindowSeconds,omitempty"`
	Policies                   []ScalingPolicy `json:"policies,omitempty"`
}

// 缩放政策界定了单一缩放政策.
type ScalingPolicy struct {
	Type          string `json:"type"` // Pods, Percent
	Value         int32  `json:"value"`
	PeriodSeconds int32  `json:"periodSeconds"`
}

// 健康检查Spec定义健康检查配置.
type HealthCheckSpec struct {
	Enabled             bool          `json:"enabled"`
	Interval            time.Duration `json:"interval"`
	Timeout             time.Duration `json:"timeout"`
	FailureThreshold    int32         `json:"failureThreshold"`
	SuccessThreshold    int32         `json:"successThreshold"`
	InitialDelaySeconds int32         `json:"initialDelaySeconds,omitempty"`
}

// AgentCRD status 代表被观察到的特工状态.
type AgentCRDStatus struct {
	Phase              AgentPhase       `json:"phase"`
	Replicas           int32            `json:"replicas"`
	ReadyReplicas      int32            `json:"readyReplicas"`
	AvailableReplicas  int32            `json:"availableReplicas"`
	Conditions         []AgentCondition `json:"conditions,omitempty"`
	LastScaleTime      *time.Time       `json:"lastScaleTime,omitempty"`
	CurrentMetrics     []MetricValue    `json:"currentMetrics,omitempty"`
	ObservedGeneration int64            `json:"observedGeneration,omitempty"`
}

// 代理阶段代表代理阶段.
type AgentPhase string

const (
	AgentPhasePending     AgentPhase = "Pending"
	AgentPhaseRunning     AgentPhase = "Running"
	AgentPhaseScaling     AgentPhase = "Scaling"
	AgentPhaseDegraded    AgentPhase = "Degraded"
	AgentPhaseFailed      AgentPhase = "Failed"
	AgentPhaseTerminating AgentPhase = "Terminating"
)

// 代理条件代表着代理的条件.
type AgentCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"` // True, False, Unknown
	LastTransitionTime time.Time `json:"lastTransitionTime"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
}

// MetricValue 代表当前计量值。
type MetricValue struct {
	Name         string `json:"name"`
	CurrentValue int64  `json:"currentValue"`
	TargetValue  int64  `json:"targetValue"`
}

// 操作员Config 配置 Kubernetes 操作员 。
type OperatorConfig struct {
	Namespace               string        `json:"namespace"`
	ReconcileInterval       time.Duration `json:"reconcileInterval"`
	MetricsPort             int           `json:"metricsPort"`
	HealthProbePort         int           `json:"healthProbePort"`
	LeaderElection          bool          `json:"leaderElection"`
	LeaderElectionID        string        `json:"leaderElectionId"`
	WatchNamespaces         []string      `json:"watchNamespaces,omitempty"`
	EnableWebhooks          bool          `json:"enableWebhooks"`
	CertDir                 string        `json:"certDir,omitempty"`
	MaxConcurrentReconciles int           `json:"maxConcurrentReconciles"`
}

// 默认操作器 Config 返回合理的默认值 。
func DefaultOperatorConfig() OperatorConfig {
	return OperatorConfig{
		Namespace:               "default",
		ReconcileInterval:       30 * time.Second,
		MetricsPort:             8080,
		HealthProbePort:         8081,
		LeaderElection:          true,
		LeaderElectionID:        "agentflow-operator-leader",
		EnableWebhooks:          false,
		MaxConcurrentReconciles: 3,
	}
}

// AgentOperator为特工执行Kubernetes操作器.
type AgentOperator struct {
	config    OperatorConfig
	agents    map[string]*AgentCRD
	instances map[string]*AgentInstance
	metrics   *OperatorMetrics
	logger    *zap.Logger
	mu        sync.RWMutex

	// 回电
	onReconcile   func(agent *AgentCRD) error
	onScale       func(agent *AgentCRD, replicas int32) error
	onHealthCheck func(agent *AgentCRD) (bool, error)

	// 控制权
	stopCh  chan struct{}
	running bool
}

// 代理Instance代表运行的代理实例.
type AgentInstance struct {
	ID          string            `json:"id"`
	AgentName   string            `json:"agentName"`
	Namespace   string            `json:"namespace"`
	Status      InstanceStatus    `json:"status"`
	StartTime   time.Time         `json:"startTime"`
	LastHealthy time.Time         `json:"lastHealthy"`
	Metrics     InstanceMetrics   `json:"metrics"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// 案件状况代表代理人案件的状况。
type InstanceStatus string

const (
	InstanceStatusPending   InstanceStatus = "Pending"
	InstanceStatusRunning   InstanceStatus = "Running"
	InstanceStatusHealthy   InstanceStatus = "Healthy"
	InstanceStatusUnhealthy InstanceStatus = "Unhealthy"
	InstanceStatusFailed    InstanceStatus = "Failed"
)

// 实例计量包含一个代理实例的衡量标准。
type InstanceMetrics struct {
	RequestsTotal     int64         `json:"requestsTotal"`
	RequestsPerSecond float64       `json:"requestsPerSecond"`
	AverageLatency    time.Duration `json:"averageLatency"`
	ErrorRate         float64       `json:"errorRate"`
	CPUUsage          float64       `json:"cpuUsage"`
	MemoryUsage       float64       `json:"memoryUsage"`
	TokensUsed        int64         `json:"tokensUsed"`
}

// 运算仪包含运算器级的度量衡.
type OperatorMetrics struct {
	ReconcileTotal       int64         `json:"reconcileTotal"`
	ReconcileErrors      int64         `json:"reconcileErrors"`
	ScaleUpEvents        int64         `json:"scaleUpEvents"`
	ScaleDownEvents      int64         `json:"scaleDownEvents"`
	SelfHealingEvents    int64         `json:"selfHealingEvents"`
	AverageReconcileTime time.Duration `json:"averageReconcileTime"`
}

// 新代理运营商创建了新的代理运营商.
func NewAgentOperator(config OperatorConfig, logger *zap.Logger) *AgentOperator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentOperator{
		config:    config,
		agents:    make(map[string]*AgentCRD),
		instances: make(map[string]*AgentInstance),
		metrics:   &OperatorMetrics{},
		logger:    logger,
		stopCh:    make(chan struct{}),
	}
}

// 设置调和 Callback 设置调和调回调 。
func (o *AgentOperator) SetReconcileCallback(fn func(agent *AgentCRD) error) {
	o.onReconcile = fn
}

// 设置缩放Callback 设置缩放回调 。
func (o *AgentOperator) SetScaleCallback(fn func(agent *AgentCRD, replicas int32) error) {
	o.onScale = fn
}

// SetHealthCheckCallback 设置健康检查回调.
func (o *AgentOperator) SetHealthCheckCallback(fn func(agent *AgentCRD) (bool, error)) {
	o.onHealthCheck = fn
}

// 启动操作员 。
func (o *AgentOperator) Start(ctx context.Context) error {
	o.mu.Lock()
	if o.running {
		o.mu.Unlock()
		return fmt.Errorf("operator already running")
	}
	o.running = true
	o.stopCh = make(chan struct{})
	o.mu.Unlock()

	o.logger.Info("starting agent operator",
		zap.String("namespace", o.config.Namespace),
		zap.Duration("reconcileInterval", o.config.ReconcileInterval))

	// 开始调节循环
	go o.reconcileLoop(ctx)

	// 开始健康检查循环
	go o.healthCheckLoop(ctx)

	// 开始收集衡量标准
	go o.metricsLoop(ctx)

	return nil
}

// 停止停止操作员。
func (o *AgentOperator) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()

	if !o.running {
		return
	}

	close(o.stopCh)
	o.running = false
	o.logger.Info("agent operator stopped")
}

// 注册代理注册代理CRD.
func (o *AgentOperator) RegisterAgent(agent *AgentCRD) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	key := fmt.Sprintf("%s/%s", agent.Metadata.Namespace, agent.Metadata.Name)

	// 初始状态
	agent.Status = AgentCRDStatus{
		Phase:    AgentPhasePending,
		Replicas: 0,
	}

	o.agents[key] = agent
	o.logger.Info("agent registered", zap.String("key", key))

	// 触发初始调节
	go o.reconcileAgent(agent)

	return nil
}

// 未注册代理删除代理CRD.
func (o *AgentOperator) UnregisterAgent(namespace, name string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	key := fmt.Sprintf("%s/%s", namespace, name)
	agent, ok := o.agents[key]
	if !ok {
		return fmt.Errorf("agent not found: %s", key)
	}

	agent.Status.Phase = AgentPhaseTerminating

	// 删除实例
	for id, inst := range o.instances {
		if inst.AgentName == name && inst.Namespace == namespace {
			delete(o.instances, id)
		}
	}

	delete(o.agents, key)
	o.logger.Info("agent unregistered", zap.String("key", key))
	return nil
}

// Get Agent 以命名空间和名称检索代理 。
func (o *AgentOperator) GetAgent(namespace, name string) *AgentCRD {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.agents[fmt.Sprintf("%s/%s", namespace, name)]
}

// ListAgents列出所有注册代理.
func (o *AgentOperator) ListAgents() []*AgentCRD {
	o.mu.RLock()
	defer o.mu.RUnlock()

	agents := make([]*AgentCRD, 0, len(o.agents))
	for _, a := range o.agents {
		agents = append(agents, a)
	}
	return agents
}

func (o *AgentOperator) reconcileLoop(ctx context.Context) {
	ticker := time.NewTicker(o.config.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.reconcileAll()
		}
	}
}

func (o *AgentOperator) reconcileAll() {
	o.mu.RLock()
	agents := make([]*AgentCRD, 0, len(o.agents))
	for _, a := range o.agents {
		agents = append(agents, a)
	}
	o.mu.RUnlock()

	for _, agent := range agents {
		o.reconcileAgent(agent)
	}
}

func (o *AgentOperator) reconcileAgent(agent *AgentCRD) {
	start := time.Now()
	o.metrics.ReconcileTotal++

	o.logger.Debug("reconciling agent",
		zap.String("name", agent.Metadata.Name),
		zap.String("namespace", agent.Metadata.Namespace))

	// 调用自定义调试回调
	if o.onReconcile != nil {
		if err := o.onReconcile(agent); err != nil {
			o.metrics.ReconcileErrors++
			o.logger.Error("reconcile callback failed", zap.Error(err))
			o.updateAgentCondition(agent, "Reconciled", "False", "ReconcileFailed", err.Error())
			return
		}
	}

	// 检查想要的复制品与实际复制品
	o.mu.RLock()
	currentReplicas := o.countInstances(agent.Metadata.Namespace, agent.Metadata.Name)
	o.mu.RUnlock()

	desiredReplicas := agent.Spec.Replicas

	// 处理缩放
	if agent.Spec.Scaling.Enabled {
		desiredReplicas = o.calculateDesiredReplicas(agent, currentReplicas)
	}

	if currentReplicas != desiredReplicas {
		o.scaleAgent(agent, desiredReplicas)
	}

	// 更新状态
	o.mu.Lock()
	agent.Status.Replicas = currentReplicas
	agent.Status.ReadyReplicas = o.countReadyInstances(agent.Metadata.Namespace, agent.Metadata.Name)
	agent.Status.AvailableReplicas = agent.Status.ReadyReplicas
	agent.Status.ObservedGeneration = agent.Metadata.Generation

	if agent.Status.ReadyReplicas == agent.Spec.Replicas {
		agent.Status.Phase = AgentPhaseRunning
	} else if agent.Status.ReadyReplicas > 0 {
		agent.Status.Phase = AgentPhaseDegraded
	}
	o.mu.Unlock()

	o.updateAgentCondition(agent, "Reconciled", "True", "ReconcileSucceeded", "")

	elapsed := time.Since(start)
	o.logger.Debug("reconcile completed",
		zap.String("name", agent.Metadata.Name),
		zap.Duration("duration", elapsed))
}

func (o *AgentOperator) calculateDesiredReplicas(agent *AgentCRD, current int32) int32 {
	if !agent.Spec.Scaling.Enabled {
		return agent.Spec.Replicas
	}

	desired := current

	// 评价尺度
	for _, metric := range agent.Spec.Scaling.TargetMetrics {
		currentValue := o.getCurrentMetricValue(agent, metric.Name)

		if currentValue > metric.TargetValue {
			// 缩放
			ratio := float64(currentValue) / float64(metric.TargetValue)
			newDesired := int32(float64(current) * ratio)
			if newDesired > desired {
				desired = newDesired
			}
		} else if currentValue < metric.TargetValue/2 {
			// 缩放( 如果明显低于目标)
			ratio := float64(currentValue) / float64(metric.TargetValue)
			newDesired := int32(float64(current) * ratio)
			if newDesired < desired && newDesired >= agent.Spec.Scaling.MinReplicas {
				desired = newDesired
			}
		}
	}

	// 应用界限
	if desired < agent.Spec.Scaling.MinReplicas {
		desired = agent.Spec.Scaling.MinReplicas
	}
	if desired > agent.Spec.Scaling.MaxReplicas {
		desired = agent.Spec.Scaling.MaxReplicas
	}

	return desired
}

func (o *AgentOperator) getCurrentMetricValue(agent *AgentCRD, metricName string) int64 {
	// 实例的总指标
	o.mu.RLock()
	defer o.mu.RUnlock()

	var total int64
	var count int64

	for _, inst := range o.instances {
		if inst.AgentName == agent.Metadata.Name && inst.Namespace == agent.Metadata.Namespace {
			switch metricName {
			case "requests_per_second":
				total += int64(inst.Metrics.RequestsPerSecond)
			case "latency":
				total += inst.Metrics.AverageLatency.Milliseconds()
			case "cpu":
				total += int64(inst.Metrics.CPUUsage * 100)
			case "memory":
				total += int64(inst.Metrics.MemoryUsage * 100)
			}
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / count
}

func (o *AgentOperator) scaleAgent(agent *AgentCRD, replicas int32) {
	o.mu.Lock()
	currentReplicas := o.countInstances(agent.Metadata.Namespace, agent.Metadata.Name)
	o.mu.Unlock()

	if replicas > currentReplicas {
		// 缩放
		o.metrics.ScaleUpEvents++
		o.logger.Info("scaling up agent",
			zap.String("name", agent.Metadata.Name),
			zap.Int32("from", currentReplicas),
			zap.Int32("to", replicas))

		for i := currentReplicas; i < replicas; i++ {
			o.createInstance(agent)
		}
	} else if replicas < currentReplicas {
		// 缩放
		o.metrics.ScaleDownEvents++
		o.logger.Info("scaling down agent",
			zap.String("name", agent.Metadata.Name),
			zap.Int32("from", currentReplicas),
			zap.Int32("to", replicas))

		o.removeInstances(agent, currentReplicas-replicas)
	}

	// 调用缩放回调
	if o.onScale != nil {
		if err := o.onScale(agent, replicas); err != nil {
			o.logger.Error("scale callback failed", zap.Error(err))
		}
	}

	now := time.Now()
	agent.Status.LastScaleTime = &now
}

func (o *AgentOperator) createInstance(agent *AgentCRD) {
	o.mu.Lock()
	defer o.mu.Unlock()

	inst := &AgentInstance{
		ID:        fmt.Sprintf("%s-%s-%d", agent.Metadata.Namespace, agent.Metadata.Name, time.Now().UnixNano()),
		AgentName: agent.Metadata.Name,
		Namespace: agent.Metadata.Namespace,
		Status:    InstanceStatusPending,
		StartTime: time.Now(),
		Labels:    agent.Metadata.Labels,
	}

	o.instances[inst.ID] = inst
	o.logger.Debug("instance created", zap.String("id", inst.ID))
}

func (o *AgentOperator) removeInstances(agent *AgentCRD, count int32) {
	o.mu.Lock()
	defer o.mu.Unlock()

	var removed int32
	for id, inst := range o.instances {
		if inst.AgentName == agent.Metadata.Name && inst.Namespace == agent.Metadata.Namespace {
			delete(o.instances, id)
			removed++
			o.logger.Debug("instance removed", zap.String("id", id))
			if removed >= count {
				break
			}
		}
	}
}

func (o *AgentOperator) countInstances(namespace, name string) int32 {
	var count int32
	for _, inst := range o.instances {
		if inst.AgentName == name && inst.Namespace == namespace {
			count++
		}
	}
	return count
}

func (o *AgentOperator) countReadyInstances(namespace, name string) int32 {
	var count int32
	for _, inst := range o.instances {
		if inst.AgentName == name && inst.Namespace == namespace {
			if inst.Status == InstanceStatusRunning || inst.Status == InstanceStatusHealthy {
				count++
			}
		}
	}
	return count
}

func (o *AgentOperator) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.checkAllHealth()
		}
	}
}

func (o *AgentOperator) checkAllHealth() {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, inst := range o.instances {
		agent := o.agents[fmt.Sprintf("%s/%s", inst.Namespace, inst.AgentName)]
		if agent == nil || !agent.Spec.HealthCheck.Enabled {
			continue
		}

		healthy := o.checkInstanceHealth(inst, agent)
		if healthy {
			inst.Status = InstanceStatusHealthy
			inst.LastHealthy = time.Now()
		} else {
			inst.Status = InstanceStatusUnhealthy
			// 自愈:重启不健康事件
			if time.Since(inst.LastHealthy) > agent.Spec.HealthCheck.Timeout*time.Duration(agent.Spec.HealthCheck.FailureThreshold) {
				o.selfHeal(inst, agent)
			}
		}
	}
}

func (o *AgentOperator) checkInstanceHealth(inst *AgentInstance, agent *AgentCRD) bool {
	if o.onHealthCheck != nil {
		healthy, err := o.onHealthCheck(agent)
		if err != nil {
			o.logger.Warn("health check failed", zap.String("instance", inst.ID), zap.Error(err))
			return false
		}
		return healthy
	}

	// 默认健康检查: 实例正在运行并有最近的活动
	if inst.Status == InstanceStatusFailed {
		return false
	}
	return time.Since(inst.StartTime) < 5*time.Minute || inst.Metrics.RequestsTotal > 0
}

func (o *AgentOperator) selfHeal(inst *AgentInstance, agent *AgentCRD) {
	o.metrics.SelfHealingEvents++
	o.logger.Info("self-healing instance",
		zap.String("instance", inst.ID),
		zap.String("agent", agent.Metadata.Name))

	// 标记为失败并创建替换
	inst.Status = InstanceStatusFailed

	// 创建替换实例
	newInst := &AgentInstance{
		ID:        fmt.Sprintf("%s-%s-%d", agent.Metadata.Namespace, agent.Metadata.Name, time.Now().UnixNano()),
		AgentName: agent.Metadata.Name,
		Namespace: agent.Metadata.Namespace,
		Status:    InstanceStatusPending,
		StartTime: time.Now(),
		Labels:    agent.Metadata.Labels,
	}
	o.instances[newInst.ID] = newInst

	// 删除失败实例
	delete(o.instances, inst.ID)

	o.updateAgentCondition(agent, "SelfHealed", "True", "InstanceReplaced",
		fmt.Sprintf("Replaced unhealthy instance %s", inst.ID))
}

func (o *AgentOperator) metricsLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.stopCh:
			return
		case <-ticker.C:
			o.collectMetrics()
		}
	}
}

func (o *AgentOperator) collectMetrics() {
	o.mu.Lock()
	defer o.mu.Unlock()

	// 更新当前计量的代理状态
	for _, agent := range o.agents {
		var metrics []MetricValue
		for _, targetMetric := range agent.Spec.Scaling.TargetMetrics {
			currentValue := o.getCurrentMetricValueLocked(agent, targetMetric.Name)
			metrics = append(metrics, MetricValue{
				Name:         targetMetric.Name,
				CurrentValue: currentValue,
				TargetValue:  targetMetric.TargetValue,
			})
		}
		agent.Status.CurrentMetrics = metrics
	}
}

func (o *AgentOperator) getCurrentMetricValueLocked(agent *AgentCRD, metricName string) int64 {
	var total int64
	var count int64

	for _, inst := range o.instances {
		if inst.AgentName == agent.Metadata.Name && inst.Namespace == agent.Metadata.Namespace {
			switch metricName {
			case "requests_per_second":
				total += int64(inst.Metrics.RequestsPerSecond)
			case "latency":
				total += inst.Metrics.AverageLatency.Milliseconds()
			case "cpu":
				total += int64(inst.Metrics.CPUUsage * 100)
			case "memory":
				total += int64(inst.Metrics.MemoryUsage * 100)
			}
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return total / count
}

func (o *AgentOperator) updateAgentCondition(agent *AgentCRD, condType, status, reason, message string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now()
	newCondition := AgentCondition{
		Type:               condType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	// 更新或附加条件
	found := false
	for i, c := range agent.Status.Conditions {
		if c.Type == condType {
			agent.Status.Conditions[i] = newCondition
			found = true
			break
		}
	}
	if !found {
		agent.Status.Conditions = append(agent.Status.Conditions, newCondition)
	}
}

// 更新InstanceMetrics为实例更新了度量衡.
func (o *AgentOperator) UpdateInstanceMetrics(instanceID string, metrics InstanceMetrics) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if inst, ok := o.instances[instanceID]; ok {
		inst.Metrics = metrics
		if inst.Status == InstanceStatusPending {
			inst.Status = InstanceStatusRunning
		}
	}
}

// GetMetrics 返回操作员的度量衡 。
func (o *AgentOperator) GetMetrics() *OperatorMetrics {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.metrics
}

// GetInstances 为代理返回所有实例 。
func (o *AgentOperator) GetInstances(namespace, name string) []*AgentInstance {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var instances []*AgentInstance
	for _, inst := range o.instances {
		if inst.AgentName == name && inst.Namespace == namespace {
			instances = append(instances, inst)
		}
	}
	return instances
}

// ExportCRD向JSON出口一名代理CRD.
func (o *AgentOperator) ExportCRD(namespace, name string) ([]byte, error) {
	agent := o.GetAgent(namespace, name)
	if agent == nil {
		return nil, fmt.Errorf("agent not found: %s/%s", namespace, name)
	}
	return json.Marshal(agent)
}

// 进口CRD从JSON进口一个代理CRD.
func (o *AgentOperator) ImportCRD(data []byte) error {
	var agent AgentCRD
	if err := json.Unmarshal(data, &agent); err != nil {
		return fmt.Errorf("failed to unmarshal CRD: %w", err)
	}
	return o.RegisterAgent(&agent)
}

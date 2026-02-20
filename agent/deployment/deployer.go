// 套件部署为AI代理提供云部署支持.
// 执行 Google ADK 风格的一击部署到 K8s/ Serverless 。
package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 部署 目标定义了部署目标类型.
type DeploymentTarget string

const (
	TargetKubernetes DeploymentTarget = "kubernetes"
	TargetCloudRun   DeploymentTarget = "cloud_run"
	TargetLambda     DeploymentTarget = "lambda"
	TargetLocal      DeploymentTarget = "local"
)

// 部署状况是部署状况。
type DeploymentStatus string

const (
	StatusPending   DeploymentStatus = "pending"
	StatusDeploying DeploymentStatus = "deploying"
	StatusRunning   DeploymentStatus = "running"
	StatusFailed    DeploymentStatus = "failed"
	StatusStopped   DeploymentStatus = "stopped"
	StatusScaling   DeploymentStatus = "scaling"
)

// 部署是部署的代理人实例。
type Deployment struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	AgentID     string            `json:"agent_id"`
	Target      DeploymentTarget  `json:"target"`
	Status      DeploymentStatus  `json:"status"`
	Endpoint    string            `json:"endpoint,omitempty"`
	Replicas    int               `json:"replicas"`
	Config      DeploymentConfig  `json:"config"`
	Resources   ResourceConfig    `json:"resources"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	HealthCheck *HealthCheck      `json:"health_check,omitempty"`
}

// 部署配置包含部署配置。
type DeploymentConfig struct {
	Image       string            `json:"image,omitempty"`
	Port        int               `json:"port"`
	Environment map[string]string `json:"environment,omitempty"`
	Secrets     []SecretRef       `json:"secrets,omitempty"`
	AutoScale   *AutoScaleConfig  `json:"auto_scale,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
}

// 资源Config定义了资源限制.
type ResourceConfig struct {
	CPURequest    string `json:"cpu_request,omitempty"`
	CPULimit      string `json:"cpu_limit,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	MemoryLimit   string `json:"memory_limit,omitempty"`
	GPUCount      int    `json:"gpu_count,omitempty"`
}

// 秘密参考文件提到一个秘密
type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
	Env  string `json:"env"`
}

// 自动缩放配置自动缩放 。
type AutoScaleConfig struct {
	MinReplicas    int           `json:"min_replicas"`
	MaxReplicas    int           `json:"max_replicas"`
	TargetCPU      int           `json:"target_cpu_percent,omitempty"`
	TargetMemory   int           `json:"target_memory_percent,omitempty"`
	TargetQPS      int           `json:"target_qps,omitempty"`
	ScaleDownDelay time.Duration `json:"scale_down_delay,omitempty"`
}

// 健康检查定义了健康检查配置.
type HealthCheck struct {
	Path             string        `json:"path"`
	Port             int           `json:"port"`
	Interval         time.Duration `json:"interval"`
	Timeout          time.Duration `json:"timeout"`
	FailureThreshold int           `json:"failure_threshold"`
}

// 部署提供方定义了部署后端的接口.
type DeploymentProvider interface {
	Deploy(ctx context.Context, deployment *Deployment) error
	Update(ctx context.Context, deployment *Deployment) error
	Delete(ctx context.Context, deploymentID string) error
	GetStatus(ctx context.Context, deploymentID string) (*Deployment, error)
	Scale(ctx context.Context, deploymentID string, replicas int) error
	GetLogs(ctx context.Context, deploymentID string, lines int) ([]string, error)
}

// 部署人员管理代理部署。
type Deployer struct {
	providers   map[DeploymentTarget]DeploymentProvider
	deployments map[string]*Deployment
	logger      *zap.Logger
	mu          sync.RWMutex
}

// 新部署者创建了新的部署者.
func NewDeployer(logger *zap.Logger) *Deployer {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Deployer{
		providers:   make(map[DeploymentTarget]DeploymentProvider),
		deployments: make(map[string]*Deployment),
		logger:      logger.With(zap.String("component", "deployer")),
	}
}

// 提供人员登记部署提供者。
func (d *Deployer) RegisterProvider(target DeploymentTarget, provider DeploymentProvider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providers[target] = provider
}

// 向指定目标部署特工。
func (d *Deployer) Deploy(ctx context.Context, opts DeployOptions) (*Deployment, error) {
	provider, ok := d.providers[opts.Target]
	if !ok {
		return nil, fmt.Errorf("no provider for target: %s", opts.Target)
	}

	deployment := &Deployment{
		ID:        generateDeploymentID(),
		Name:      opts.Name,
		AgentID:   opts.AgentID,
		Target:    opts.Target,
		Status:    StatusPending,
		Replicas:  opts.Replicas,
		Config:    opts.Config,
		Resources: opts.Resources,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Metadata:  opts.Metadata,
	}

	if deployment.Replicas == 0 {
		deployment.Replicas = 1
	}

	d.logger.Info("deploying agent",
		zap.String("id", deployment.ID),
		zap.String("agent_id", opts.AgentID),
		zap.String("target", string(opts.Target)),
	)

	deployment.Status = StatusDeploying
	d.mu.Lock()
	d.deployments[deployment.ID] = deployment
	d.mu.Unlock()

	if err := provider.Deploy(ctx, deployment); err != nil {
		deployment.Status = StatusFailed
		return deployment, fmt.Errorf("deployment failed: %w", err)
	}

	deployment.Status = StatusRunning
	deployment.UpdatedAt = time.Now()

	d.logger.Info("deployment successful",
		zap.String("id", deployment.ID),
		zap.String("endpoint", deployment.Endpoint),
	)

	return deployment, nil
}

// 更新现有部署。
func (d *Deployer) Update(ctx context.Context, deploymentID string, config DeploymentConfig) error {
	d.mu.Lock()
	deployment, ok := d.deployments[deploymentID]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("deployment not found: %s", deploymentID)
	}
	d.mu.Unlock()

	provider, ok := d.providers[deployment.Target]
	if !ok {
		return fmt.Errorf("no provider for target: %s", deployment.Target)
	}

	deployment.Config = config
	deployment.UpdatedAt = time.Now()

	return provider.Update(ctx, deployment)
}

// 删除一个部署。
func (d *Deployer) Delete(ctx context.Context, deploymentID string) error {
	d.mu.Lock()
	deployment, ok := d.deployments[deploymentID]
	if !ok {
		d.mu.Unlock()
		return fmt.Errorf("deployment not found: %s", deploymentID)
	}
	d.mu.Unlock()

	provider, ok := d.providers[deployment.Target]
	if !ok {
		return fmt.Errorf("no provider for target: %s", deployment.Target)
	}

	if err := provider.Delete(ctx, deploymentID); err != nil {
		return fmt.Errorf("delete deployment %s: %w", deploymentID, err)
	}

	d.mu.Lock()
	delete(d.deployments, deploymentID)
	d.mu.Unlock()

	d.logger.Info("deployment deleted", zap.String("id", deploymentID))
	return nil
}

// 缩放一个部署。
func (d *Deployer) Scale(ctx context.Context, deploymentID string, replicas int) error {
	d.mu.RLock()
	deployment, ok := d.deployments[deploymentID]
	d.mu.RUnlock()

	if !ok {
		return fmt.Errorf("deployment not found: %s", deploymentID)
	}

	provider, ok := d.providers[deployment.Target]
	if !ok {
		return fmt.Errorf("no provider for target: %s", deployment.Target)
	}

	d.logger.Info("scaling deployment",
		zap.String("id", deploymentID),
		zap.Int("from", deployment.Replicas),
		zap.Int("to", replicas),
	)

	if err := provider.Scale(ctx, deploymentID, replicas); err != nil {
		return fmt.Errorf("scale deployment %s: %w", deploymentID, err)
	}

	d.mu.Lock()
	deployment.Replicas = replicas
	deployment.UpdatedAt = time.Now()
	d.mu.Unlock()

	return nil
}

// 得到部署返回一个部署的ID。
func (d *Deployer) GetDeployment(deploymentID string) (*Deployment, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployment, ok := d.deployments[deploymentID]
	if !ok {
		return nil, fmt.Errorf("deployment not found: %s", deploymentID)
	}
	return deployment, nil
}

// 列表调度返回所有部署 。
func (d *Deployer) ListDeployments() []*Deployment {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []*Deployment
	for _, dep := range d.deployments {
		result = append(result, dep)
	}
	return result
}

// 部署选项配置部署 。
type DeployOptions struct {
	Name      string
	AgentID   string
	Target    DeploymentTarget
	Replicas  int
	Config    DeploymentConfig
	Resources ResourceConfig
	Metadata  map[string]string
}

func generateDeploymentID() string {
	return fmt.Sprintf("dep_%d", time.Now().UnixNano())
}

// 按 K8s 显示的“出口管理”出口部署。
func (d *Deployer) ExportManifest(deploymentID string) ([]byte, error) {
	deployment, err := d.GetDeployment(deploymentID)
	if err != nil {
		return nil, fmt.Errorf("get deployment for manifest export: %w", err)
	}

	manifest := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name": deployment.Name,
			"labels": map[string]string{
				"app":      deployment.Name,
				"agent-id": deployment.AgentID,
			},
		},
		"spec": map[string]any{
			"replicas": deployment.Replicas,
			"selector": map[string]any{
				"matchLabels": map[string]string{"app": deployment.Name},
			},
			"template": map[string]any{
				"metadata": map[string]any{
					"labels": map[string]string{"app": deployment.Name},
				},
				"spec": map[string]any{
					"containers": []map[string]any{{
						"name":  deployment.Name,
						"image": deployment.Config.Image,
						"ports": []map[string]any{{
							"containerPort": deployment.Config.Port,
						}},
						"resources": map[string]any{
							"requests": map[string]string{
								"cpu":    deployment.Resources.CPURequest,
								"memory": deployment.Resources.MemoryRequest,
							},
							"limits": map[string]string{
								"cpu":    deployment.Resources.CPULimit,
								"memory": deployment.Resources.MemoryLimit,
							},
						},
					}},
				},
			},
		},
	}

	return json.MarshalIndent(manifest, "", "  ")
}

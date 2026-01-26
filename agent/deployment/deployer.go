// Package deployment provides cloud deployment support for AI agents.
// Implements Google ADK-style one-click deployment to K8s/Serverless.
package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DeploymentTarget defines the deployment target type.
type DeploymentTarget string

const (
	TargetKubernetes DeploymentTarget = "kubernetes"
	TargetCloudRun   DeploymentTarget = "cloud_run"
	TargetLambda     DeploymentTarget = "lambda"
	TargetLocal      DeploymentTarget = "local"
)

// DeploymentStatus represents deployment status.
type DeploymentStatus string

const (
	StatusPending   DeploymentStatus = "pending"
	StatusDeploying DeploymentStatus = "deploying"
	StatusRunning   DeploymentStatus = "running"
	StatusFailed    DeploymentStatus = "failed"
	StatusStopped   DeploymentStatus = "stopped"
	StatusScaling   DeploymentStatus = "scaling"
)

// Deployment represents a deployed agent instance.
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

// DeploymentConfig contains deployment configuration.
type DeploymentConfig struct {
	Image       string            `json:"image,omitempty"`
	Port        int               `json:"port"`
	Environment map[string]string `json:"environment,omitempty"`
	Secrets     []SecretRef       `json:"secrets,omitempty"`
	AutoScale   *AutoScaleConfig  `json:"auto_scale,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
}

// ResourceConfig defines resource limits.
type ResourceConfig struct {
	CPURequest    string `json:"cpu_request,omitempty"`
	CPULimit      string `json:"cpu_limit,omitempty"`
	MemoryRequest string `json:"memory_request,omitempty"`
	MemoryLimit   string `json:"memory_limit,omitempty"`
	GPUCount      int    `json:"gpu_count,omitempty"`
}

// SecretRef references a secret.
type SecretRef struct {
	Name string `json:"name"`
	Key  string `json:"key"`
	Env  string `json:"env"`
}

// AutoScaleConfig configures auto-scaling.
type AutoScaleConfig struct {
	MinReplicas    int           `json:"min_replicas"`
	MaxReplicas    int           `json:"max_replicas"`
	TargetCPU      int           `json:"target_cpu_percent,omitempty"`
	TargetMemory   int           `json:"target_memory_percent,omitempty"`
	TargetQPS      int           `json:"target_qps,omitempty"`
	ScaleDownDelay time.Duration `json:"scale_down_delay,omitempty"`
}

// HealthCheck defines health check configuration.
type HealthCheck struct {
	Path             string        `json:"path"`
	Port             int           `json:"port"`
	Interval         time.Duration `json:"interval"`
	Timeout          time.Duration `json:"timeout"`
	FailureThreshold int           `json:"failure_threshold"`
}

// DeploymentProvider defines the interface for deployment backends.
type DeploymentProvider interface {
	Deploy(ctx context.Context, deployment *Deployment) error
	Update(ctx context.Context, deployment *Deployment) error
	Delete(ctx context.Context, deploymentID string) error
	GetStatus(ctx context.Context, deploymentID string) (*Deployment, error)
	Scale(ctx context.Context, deploymentID string, replicas int) error
	GetLogs(ctx context.Context, deploymentID string, lines int) ([]string, error)
}

// Deployer manages agent deployments.
type Deployer struct {
	providers   map[DeploymentTarget]DeploymentProvider
	deployments map[string]*Deployment
	logger      *zap.Logger
	mu          sync.RWMutex
}

// NewDeployer creates a new deployer.
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

// RegisterProvider registers a deployment provider.
func (d *Deployer) RegisterProvider(target DeploymentTarget, provider DeploymentProvider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providers[target] = provider
}

// Deploy deploys an agent to the specified target.
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

// Update updates an existing deployment.
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

// Delete removes a deployment.
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
		return err
	}

	d.mu.Lock()
	delete(d.deployments, deploymentID)
	d.mu.Unlock()

	d.logger.Info("deployment deleted", zap.String("id", deploymentID))
	return nil
}

// Scale scales a deployment.
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
		return err
	}

	d.mu.Lock()
	deployment.Replicas = replicas
	deployment.UpdatedAt = time.Now()
	d.mu.Unlock()

	return nil
}

// GetDeployment returns a deployment by ID.
func (d *Deployer) GetDeployment(deploymentID string) (*Deployment, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	deployment, ok := d.deployments[deploymentID]
	if !ok {
		return nil, fmt.Errorf("deployment not found: %s", deploymentID)
	}
	return deployment, nil
}

// ListDeployments returns all deployments.
func (d *Deployer) ListDeployments() []*Deployment {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []*Deployment
	for _, dep := range d.deployments {
		result = append(result, dep)
	}
	return result
}

// DeployOptions configures deployment.
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

// ExportManifest exports deployment as K8s manifest.
func (d *Deployer) ExportManifest(deploymentID string) ([]byte, error) {
	deployment, err := d.GetDeployment(deploymentID)
	if err != nil {
		return nil, err
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

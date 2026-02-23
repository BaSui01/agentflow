package deployment

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"go.uber.org/zap"
)

// DockerProvider manages agents as Docker containers.
// It uses exec.Command("docker", ...) — no Docker SDK dependency.
type DockerProvider struct {
	logger *zap.Logger
}

// NewDockerProvider creates a new Docker provider.
func NewDockerProvider(logger *zap.Logger) *DockerProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DockerProvider{
		logger: logger.With(zap.String("provider", "docker")),
	}
}

// containerName derives a deterministic container name from a deployment.
func containerName(d *Deployment) string {
	return fmt.Sprintf("agentflow-%s", d.ID)
}

// Deploy starts a Docker container for the deployment.
func (p *DockerProvider) Deploy(ctx context.Context, d *Deployment) error {
	if d.Config.Image == "" {
		return fmt.Errorf("docker provider requires Config.Image")
	}

	name := containerName(d)
	port := d.Config.Port
	if port == 0 {
		port = 8080
	}

	args := []string{
		"run", "-d",
		"--name", name,
		"-p", fmt.Sprintf("%d:%d", port, port),
	}

	for k, v := range d.Config.Environment {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, d.Config.Image)

	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %w: %s", err, string(out))
	}

	d.Endpoint = fmt.Sprintf("http://127.0.0.1:%d", port)
	p.logger.Info("docker container started",
		zap.String("id", d.ID),
		zap.String("container", name))
	return nil
}

// Update stops the old container and starts a new one.
func (p *DockerProvider) Update(ctx context.Context, d *Deployment) error {
	if err := p.Delete(ctx, d.ID); err != nil {
		p.logger.Warn("failed to remove old container during update", zap.Error(err))
	}
	return p.Deploy(ctx, d)
}

// Delete stops and removes the Docker container.
func (p *DockerProvider) Delete(ctx context.Context, deploymentID string) error {
	name := fmt.Sprintf("agentflow-%s", deploymentID)

	stopOut, err := exec.CommandContext(ctx, "docker", "stop", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop failed: %w: %s", err, string(stopOut))
	}

	rmOut, err := exec.CommandContext(ctx, "docker", "rm", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm failed: %w: %s", err, string(rmOut))
	}

	p.logger.Info("docker container removed", zap.String("container", name))
	return nil
}

// GetStatus inspects the Docker container state.
func (p *DockerProvider) GetStatus(ctx context.Context, deploymentID string) (*Deployment, error) {
	name := fmt.Sprintf("agentflow-%s", deploymentID)

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{.State.Status}}", name)
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return &Deployment{ID: deploymentID, Status: StatusStopped}, nil
	}

	state := strings.TrimSpace(out.String())
	status := StatusRunning
	switch state {
	case "running":
		status = StatusRunning
	case "exited", "dead":
		status = StatusStopped
	case "created", "restarting":
		status = StatusPending
	default:
		status = StatusFailed
	}

	return &Deployment{ID: deploymentID, Status: status}, nil
}

// Scale is not supported for single Docker containers.
func (p *DockerProvider) Scale(_ context.Context, deploymentID string, replicas int) error {
	return fmt.Errorf("docker provider does not support scaling deployment %s to %d replicas", deploymentID, replicas)
}

// GetLogs returns recent log lines from the Docker container.
func (p *DockerProvider) GetLogs(ctx context.Context, deploymentID string, lines int) ([]string, error) {
	name := fmt.Sprintf("agentflow-%s", deploymentID)

	if lines <= 0 {
		lines = 100
	}

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", "logs",
		"--tail", fmt.Sprintf("%d", lines), name)
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("docker logs failed: %w", err)
	}

	return splitDockerLines(out.String()), nil
}

// splitDockerLines splits output into non-empty lines.
func splitDockerLines(s string) []string {
	raw := strings.Split(s, "\n")
	var result []string
	for _, line := range raw {
		trimmed := strings.TrimRight(line, "\r")
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

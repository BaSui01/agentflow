package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Registry manages service registration, startup, and shutdown.
type Registry struct {
	services []serviceEntry
	logger   *zap.Logger
	mu       sync.Mutex
}

type serviceEntry struct {
	service Service
	info    ServiceInfo
	started bool
}

// NewRegistry creates a new service registry.
func NewRegistry(logger *zap.Logger) *Registry {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Registry{
		services: make([]serviceEntry, 0),
		logger:   logger.With(zap.String("component", "service_registry")),
	}
}

// Register adds a service to the registry.
func (r *Registry) Register(svc Service, info ServiceInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.services = append(r.services, serviceEntry{
		service: svc,
		info:    info,
	})
}

// StartAll starts all registered services in dependency/priority order.
// If any service fails to start, already-started services are stopped in reverse order.
func (r *Registry) StartAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ordered, err := r.topologicalSort()
	if err != nil {
		return fmt.Errorf("service dependency resolution failed: %w", err)
	}

	for i, idx := range ordered {
		entry := &r.services[idx]
		name := entry.info.Name

		// Skip already-started services (supports incremental startup).
		if entry.started {
			continue
		}

		// Verify dependencies are started.
		for _, dep := range entry.info.DependsOn {
			if !r.isStartedLocked(dep) {
				// Roll back already-started services.
				r.stopStartedLocked(ctx, ordered[:i])
				return fmt.Errorf("service %q depends on %q which is not started", name, dep)
			}
		}

		r.logger.Info("starting service", zap.String("service", name), zap.Int("priority", entry.info.Priority))
		if err := entry.service.Start(ctx); err != nil {
			r.logger.Error("service start failed", zap.String("service", name), zap.Error(err))
			r.stopStartedLocked(ctx, ordered[:i])
			return fmt.Errorf("failed to start service %q: %w", name, err)
		}
		entry.started = true
		r.logger.Info("service started", zap.String("service", name))
	}

	return nil
}

// StopAll stops all started services in reverse startup order.
// Each service Stop call has a 10-second timeout.
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ordered, err := r.topologicalSort()
	if err != nil {
		// Best effort: stop in reverse registration order.
		for i := len(r.services) - 1; i >= 0; i-- {
			r.stopOneLocked(ctx, &r.services[i])
		}
		return nil
	}

	r.stopStartedLocked(ctx, ordered)
	return nil
}

// HealthChecks returns all registered services that implement HealthChecker.
func (r *Registry) HealthChecks() []HealthChecker {
	r.mu.Lock()
	defer r.mu.Unlock()

	var checks []HealthChecker
	for _, entry := range r.services {
		if hc, ok := entry.service.(HealthChecker); ok {
			checks = append(checks, hc)
		}
	}
	return checks
}

// stopStartedLocked stops services in reverse order of the given index slice.
// Must be called with r.mu held.
func (r *Registry) stopStartedLocked(ctx context.Context, orderedIdxs []int) {
	for i := len(orderedIdxs) - 1; i >= 0; i-- {
		entry := &r.services[orderedIdxs[i]]
		r.stopOneLocked(ctx, entry)
	}
}

// stopOneLocked stops a single service with a 10-second timeout.
func (r *Registry) stopOneLocked(ctx context.Context, entry *serviceEntry) {
	if !entry.started {
		return
	}
	name := entry.info.Name
	r.logger.Info("stopping service", zap.String("service", name))

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := entry.service.Stop(stopCtx); err != nil {
		r.logger.Error("service stop failed", zap.String("service", name), zap.Error(err))
	} else {
		r.logger.Info("service stopped", zap.String("service", name))
	}
	entry.started = false
}

// isStartedLocked checks if a service with the given name is started.
func (r *Registry) isStartedLocked(name string) bool {
	for _, entry := range r.services {
		if entry.info.Name == name && entry.started {
			return true
		}
	}
	return false
}

// topologicalSort returns service indices sorted by priority, respecting DependsOn.
// Dependencies always come before dependents regardless of priority.
func (r *Registry) topologicalSort() ([]int, error) {
	n := len(r.services)
	if n == 0 {
		return nil, nil
	}

	// Build name -> index map.
	nameIdx := make(map[string]int, n)
	for i, entry := range r.services {
		nameIdx[entry.info.Name] = i
	}

	// Sort by priority first, then resolve dependencies via stable topological sort.
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		return r.services[indices[a]].info.Priority < r.services[indices[b]].info.Priority
	})

	// Kahn's algorithm for topological sort.
	inDegree := make([]int, n)
	adj := make([][]int, n)
	for i := range adj {
		adj[i] = make([]int, 0)
	}

	for i, entry := range r.services {
		for _, dep := range entry.info.DependsOn {
			depIdx, ok := nameIdx[dep]
			if !ok {
				return nil, fmt.Errorf("service %q depends on unknown service %q", entry.info.Name, dep)
			}
			adj[depIdx] = append(adj[depIdx], i)
			inDegree[i]++
		}
	}

	// Use a priority-aware queue: among nodes with inDegree 0, pick lowest priority first.
	var queue []int
	for _, idx := range indices {
		if inDegree[idx] == 0 {
			queue = append(queue, idx)
		}
	}

	var result []int
	for len(queue) > 0 {
		// Pick the first (lowest priority) from queue.
		curr := queue[0]
		queue = queue[1:]
		result = append(result, curr)

		for _, next := range adj[curr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				// Insert maintaining priority order.
				inserted := false
				for j, q := range queue {
					if r.services[next].info.Priority < r.services[q].info.Priority {
						queue = append(queue[:j+1], queue[j:]...)
						queue[j] = next
						inserted = true
						break
					}
				}
				if !inserted {
					queue = append(queue, next)
				}
			}
		}
	}

	if len(result) != n {
		return nil, fmt.Errorf("circular dependency detected among services")
	}

	return result, nil
}

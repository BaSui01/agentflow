package federation

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/internal/tlsutil"
	"go.uber.org/zap"
)

// 联邦 节点代表联邦的一个节点.
type FederatedNode struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Endpoint     string            `json:"endpoint"`
	PublicKey    string            `json:"public_key,omitempty"`
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Status       NodeStatus        `json:"status"`
	LastSeen     time.Time         `json:"last_seen"`
}

// 节点状态代表一个联邦节点的地位.
type NodeStatus string

const (
	NodeStatusOnline   NodeStatus = "online"
	NodeStatusOffline  NodeStatus = "offline"
	NodeStatusDegraded NodeStatus = "degraded"
)

// 联邦 这项任务是分配给各联邦的任务。
type FederatedTask struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Payload      any            `json:"payload"`
	SourceNode   string         `json:"source_node"`
	TargetNodes  []string       `json:"target_nodes,omitempty"`
	Priority     int            `json:"priority"`
	Timeout      time.Duration  `json:"timeout"`
	RequiredCaps []string       `json:"required_capabilities,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	Status       TaskStatus     `json:"status"`
	Results      map[string]any `json:"results,omitempty"`
}

// 任务状态代表联合任务状态.
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
)

// Federation Config配置了联邦管弦乐团.
type FederationConfig struct {
	NodeID            string
	NodeName          string
	ListenAddr        string
	TLSConfig         *tls.Config
	HeartbeatInterval time.Duration
	TaskTimeout       time.Duration
}

// Orchestrator管理联邦代理合作.
type Orchestrator struct {
	config     FederationConfig
	nodes      map[string]*FederatedNode
	tasks      map[string]*FederatedTask
	handlers   map[string]TaskHandler
	httpClient *http.Client
	logger     *zap.Logger
	mu         sync.RWMutex
	done       chan struct{}
	closeOnce  sync.Once
}

// 特劳斯·汉德勒处理联邦任务.
type TaskHandler func(ctx context.Context, task *FederatedTask) (any, error)

// 新奥尔良创造了一个新的联邦管弦乐团.
func NewOrchestrator(config FederationConfig, logger *zap.Logger) *Orchestrator {
	if logger == nil {
		logger = zap.NewNop()
	}
	if config.HeartbeatInterval == 0 {
		config.HeartbeatInterval = 30 * time.Second
	}
	if config.TaskTimeout == 0 {
		config.TaskTimeout = 5 * time.Minute
	}

	tlsCfg := config.TLSConfig
	if tlsCfg == nil {
		tlsCfg = tlsutil.DefaultTLSConfig()
	}

	return &Orchestrator{
		config:   config,
		nodes:    make(map[string]*FederatedNode),
		tasks:    make(map[string]*FederatedTask),
		handlers: make(map[string]TaskHandler),
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
		logger: logger.With(zap.String("component", "federation")),
		done:   make(chan struct{}),
	}
}

// 注册点在联邦登记一个节点。
func (o *Orchestrator) RegisterNode(node *FederatedNode) {
	o.mu.Lock()
	defer o.mu.Unlock()
	node.Status = NodeStatusOnline
	node.LastSeen = time.Now()
	o.nodes[node.ID] = node
	o.logger.Info("node registered", zap.String("node_id", node.ID))
}

// UnregisterNode从联邦中删除一个节点.
func (o *Orchestrator) UnregisterNode(nodeID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	delete(o.nodes, nodeID)
	o.logger.Info("node unregistered", zap.String("node_id", nodeID))
}

// 登记 Handler 登记任务处理器 。
func (o *Orchestrator) RegisterHandler(taskType string, handler TaskHandler) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.handlers[taskType] = handler
}

// 向联邦提交任务。
func (o *Orchestrator) SubmitTask(ctx context.Context, task *FederatedTask) error {
	task.ID = fmt.Sprintf("ftask_%d", time.Now().UnixNano())
	task.SourceNode = o.config.NodeID
	task.CreatedAt = time.Now()
	task.Status = TaskStatusPending
	task.Results = make(map[string]any)

	if task.Timeout == 0 {
		task.Timeout = o.config.TaskTimeout
	}

	o.mu.Lock()
	o.tasks[task.ID] = task
	o.mu.Unlock()

	// 找到能够的节点
	targetNodes := o.findCapableNodes(task)
	if len(targetNodes) == 0 {
		task.Status = TaskStatusFailed
		return fmt.Errorf("no capable nodes found")
	}

	task.TargetNodes = targetNodes
	task.Status = TaskStatusRunning

	// 分配任务
	go o.distributeTask(ctx, task)

	return nil
}

func (o *Orchestrator) findCapableNodes(task *FederatedTask) []string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var capable []string
	for _, node := range o.nodes {
		if node.Status != NodeStatusOnline {
			continue
		}
		if len(task.RequiredCaps) == 0 {
			capable = append(capable, node.ID)
			continue
		}
		// 检查能力
		hasAll := true
		for _, req := range task.RequiredCaps {
			found := false
			for _, cap := range node.Capabilities {
				if cap == req {
					found = true
					break
				}
			}
			if !found {
				hasAll = false
				break
			}
		}
		if hasAll {
			capable = append(capable, node.ID)
		}
	}
	return capable
}

func (o *Orchestrator) distributeTask(ctx context.Context, task *FederatedTask) {
	ctx, cancel := context.WithTimeout(ctx, task.Timeout)
	defer cancel()

	var wg sync.WaitGroup
	resultCh := make(chan struct {
		nodeID string
		result any
		err    error
	}, len(task.TargetNodes))

	for _, nodeID := range task.TargetNodes {
		wg.Add(1)
		go func(nid string) {
			defer wg.Done()
			result, err := o.executeOnNode(ctx, nid, task)
			resultCh <- struct {
				nodeID string
				result any
				err    error
			}{nid, result, err}
		}(nodeID)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for res := range resultCh {
		o.mu.Lock()
		if res.err != nil {
			task.Results[res.nodeID] = map[string]string{"error": res.err.Error()}
		} else {
			task.Results[res.nodeID] = res.result
		}
		o.mu.Unlock()
	}

	task.Status = TaskStatusCompleted
	o.logger.Info("federated task completed", zap.String("task_id", task.ID))
}

func (o *Orchestrator) executeOnNode(ctx context.Context, nodeID string, task *FederatedTask) (any, error) {
	o.mu.RLock()
	node, ok := o.nodes[nodeID]
	o.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("node not found: %s", nodeID)
	}

	// 如果本地节点, 直接执行
	if nodeID == o.config.NodeID {
		handler, ok := o.handlers[task.Type]
		if !ok {
			return nil, fmt.Errorf("no handler for task type: %s", task.Type)
		}
		return handler(ctx, task)
	}

	// 通过 HTTP 远程执行
	payload, _ := json.Marshal(task)
	req, err := http.NewRequestWithContext(ctx, "POST", node.Endpoint+"/federation/task", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	_ = payload // Would send payload in real implementation

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetTask 以 ID 检索任务 。
func (o *Orchestrator) GetTask(taskID string) (*FederatedTask, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	task, ok := o.tasks[taskID]
	return task, ok
}

// ListNodes 返回所有已注册的节点 。
func (o *Orchestrator) ListNodes() []*FederatedNode {
	o.mu.RLock()
	defer o.mu.RUnlock()
	nodes := make([]*FederatedNode, 0, len(o.nodes))
	for _, n := range o.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

// 开始指挥
func (o *Orchestrator) Start(ctx context.Context) error {
	o.logger.Info("federation orchestrator started")
	go o.heartbeatLoop(ctx)
	return nil
}

func (o *Orchestrator) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(o.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-o.done:
			return
		case <-ticker.C:
			o.checkNodeHealth()
		}
	}
}

func (o *Orchestrator) checkNodeHealth() {
	o.mu.Lock()
	defer o.mu.Unlock()

	threshold := time.Now().Add(-3 * o.config.HeartbeatInterval)
	for _, node := range o.nodes {
		if node.LastSeen.Before(threshold) {
			node.Status = NodeStatusOffline
		}
	}
}

// 停止停止指挥器。
func (o *Orchestrator) Stop() {
	o.closeOnce.Do(func() { close(o.done) })
	o.logger.Info("federation orchestrator stopped")
}

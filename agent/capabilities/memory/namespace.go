package memory

import "context"

// NamespacedManager wraps a MemoryManager and prefixes agentID with a namespace,
// ensuring sub-agent memory reads/writes are isolated from the parent agent.
type NamespacedManager struct {
	inner     MemoryManager
	namespace string
}

// NewNamespacedManager creates a NamespacedManager.
// The namespace is typically the sub-agent's agent_id.
func NewNamespacedManager(inner MemoryManager, namespace string) *NamespacedManager {
	return &NamespacedManager{inner: inner, namespace: namespace}
}

// Namespace returns the configured namespace.
func (n *NamespacedManager) Namespace() string { return n.namespace }

func (n *NamespacedManager) scopedID(agentID string) string {
	return n.namespace + "/" + agentID
}

// --- MemoryWriter ---

func (n *NamespacedManager) Save(ctx context.Context, rec MemoryRecord) error {
	rec.AgentID = n.scopedID(rec.AgentID)
	return n.inner.Save(ctx, rec)
}

func (n *NamespacedManager) Delete(ctx context.Context, id string) error {
	return n.inner.Delete(ctx, id)
}

func (n *NamespacedManager) Clear(ctx context.Context, agentID string, kind MemoryKind) error {
	return n.inner.Clear(ctx, n.scopedID(agentID), kind)
}

// --- MemoryReader ---

func (n *NamespacedManager) LoadRecent(ctx context.Context, agentID string, kind MemoryKind, limit int) ([]MemoryRecord, error) {
	return n.inner.LoadRecent(ctx, n.scopedID(agentID), kind, limit)
}

func (n *NamespacedManager) Search(ctx context.Context, agentID string, query string, topK int) ([]MemoryRecord, error) {
	return n.inner.Search(ctx, n.scopedID(agentID), query, topK)
}

func (n *NamespacedManager) Get(ctx context.Context, id string) (*MemoryRecord, error) {
	return n.inner.Get(ctx, id)
}

package memory

import (
	"context"
	"testing"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestInMemoryKnowledgeGraph_AddEntity(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "e1", Type: "concept", Name: "Go"}))
	entity, err := kg.QueryEntity(ctx, "e1")
	require.NoError(t, err)
	assert.Equal(t, "Go", entity.Name)
	assert.False(t, entity.CreatedAt.IsZero())
}

func TestInMemoryKnowledgeGraph_AddEntity_NilError(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	assert.Error(t, kg.AddEntity(context.Background(), nil))
}

func TestInMemoryKnowledgeGraph_AddEntity_EmptyID(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	assert.Error(t, kg.AddEntity(context.Background(), &Entity{}))
}

func TestInMemoryKnowledgeGraph_QueryEntity_NotFound(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	_, err := kg.QueryEntity(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestInMemoryKnowledgeGraph_QueryEntity_EmptyID(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	_, err := kg.QueryEntity(context.Background(), "")
	assert.Error(t, err)
}

func TestInMemoryKnowledgeGraph_AddRelation(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "e1", Name: "Go"}))
	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "e2", Name: "Concurrency"}))
	require.NoError(t, kg.AddRelation(ctx, &Relation{
		ID: "r1", FromID: "e1", ToID: "e2", Type: "has_feature",
	}))

	rels, err := kg.QueryRelations(ctx, "e1", "")
	require.NoError(t, err)
	assert.Len(t, rels, 1)
	assert.Equal(t, "has_feature", rels[0].Type)
}

func TestInMemoryKnowledgeGraph_AddRelation_NilError(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	assert.Error(t, kg.AddRelation(context.Background(), nil))
}

func TestInMemoryKnowledgeGraph_AddRelation_MissingIDs(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	assert.Error(t, kg.AddRelation(context.Background(), &Relation{ID: "r1", FromID: "", ToID: ""}))
}

func TestInMemoryKnowledgeGraph_QueryRelations_BothDirections(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, kg.AddRelation(ctx, &Relation{ID: "r1", FromID: "a", ToID: "b", Type: "knows"}))
	require.NoError(t, kg.AddRelation(ctx, &Relation{ID: "r2", FromID: "c", ToID: "a", Type: "likes"}))

	rels, err := kg.QueryRelations(ctx, "a", "")
	require.NoError(t, err)
	assert.Len(t, rels, 2, "should return relations where entity is source or target")
}

func TestInMemoryKnowledgeGraph_QueryRelations_FilterByType(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, kg.AddRelation(ctx, &Relation{ID: "r1", FromID: "a", ToID: "b", Type: "knows"}))
	require.NoError(t, kg.AddRelation(ctx, &Relation{ID: "r2", FromID: "a", ToID: "c", Type: "likes"}))

	rels, err := kg.QueryRelations(ctx, "a", "knows")
	require.NoError(t, err)
	assert.Len(t, rels, 1)
	assert.Equal(t, "knows", rels[0].Type)
}

func TestInMemoryKnowledgeGraph_FindPath(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "a", Name: "A"}))
	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "b", Name: "B"}))
	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "c", Name: "C"}))
	require.NoError(t, kg.AddRelation(ctx, &Relation{ID: "r1", FromID: "a", ToID: "b"}))
	require.NoError(t, kg.AddRelation(ctx, &Relation{ID: "r2", FromID: "b", ToID: "c"}))

	paths, err := kg.FindPath(ctx, "a", "c", 5)
	require.NoError(t, err)
	assert.NotEmpty(t, paths)
	assert.Equal(t, []string{"a", "b", "c"}, paths[0])
}

func TestInMemoryKnowledgeGraph_FindPath_NoPath(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "a", Name: "A"}))
	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "b", Name: "B"}))

	paths, err := kg.FindPath(ctx, "a", "b", 5)
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestInMemoryKnowledgeGraph_FindPath_ZeroDepth(t *testing.T) {
	t.Parallel()
	kg := NewInMemoryKnowledgeGraph(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "a", Name: "A"}))
	require.NoError(t, kg.AddEntity(ctx, &Entity{ID: "b", Name: "B"}))

	paths, err := kg.FindPath(ctx, "a", "b", 0)
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestInMemoryEpisodicStore_RecordQuery(t *testing.T) {
	t.Parallel()
	store := NewInMemoryEpisodicStore(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, store.RecordEvent(ctx, &types.EpisodicEvent{AgentID: "a1", Type: "action", Content: "deployed"}))
	require.NoError(t, store.RecordEvent(ctx, &types.EpisodicEvent{AgentID: "a2", Type: "action", Content: "tested"}))
	require.NoError(t, store.RecordEvent(ctx, &types.EpisodicEvent{AgentID: "a1", Type: "observation", Content: "healthy"}))

	events, err := store.QueryEvents(ctx, EpisodicQuery{AgentID: "a1"})
	require.NoError(t, err)
	assert.Len(t, events, 2)
}

func TestInMemoryEpisodicStore_QueryByType(t *testing.T) {
	t.Parallel()
	store := NewInMemoryEpisodicStore(zap.NewNop())
	ctx := context.Background()

	require.NoError(t, store.RecordEvent(ctx, &types.EpisodicEvent{AgentID: "a1", Type: "action", Content: "act1"}))
	require.NoError(t, store.RecordEvent(ctx, &types.EpisodicEvent{AgentID: "a1", Type: "observation", Content: "obs1"}))

	events, err := store.QueryEvents(ctx, EpisodicQuery{AgentID: "a1", Type: "action"})
	require.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "action", events[0].Type)
}

func TestInMemoryEpisodicStore_QueryLimit(t *testing.T) {
	t.Parallel()
	store := NewInMemoryEpisodicStore(zap.NewNop())
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		require.NoError(t, store.RecordEvent(ctx, &types.EpisodicEvent{AgentID: "a1", Type: "action", Content: "act"}))
	}

	events, err := store.QueryEvents(ctx, EpisodicQuery{AgentID: "a1", Limit: 3})
	require.NoError(t, err)
	assert.Len(t, events, 3)
}


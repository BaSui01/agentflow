package rag

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewKnowledgeGraph(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	if graph == nil {
		t.Fatal("expected graph to be created")
	}

	if graph.nodes == nil {
		t.Error("expected nodes map to be initialized")
	}

	if graph.edges == nil {
		t.Error("expected edges map to be initialized")
	}
}

func TestNewKnowledgeGraph_NilLogger(t *testing.T) {
	graph := NewKnowledgeGraph(nil)

	if graph == nil {
		t.Fatal("expected graph to be created even with nil logger")
	}
}

func TestKnowledgeGraph_AddNode(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	node := &Node{
		ID:    "node1",
		Type:  "entity",
		Label: "Test Node",
		Properties: map[string]any{
			"key": "value",
		},
	}

	graph.AddNode(node)

	// 添加了验证节点
	retrieved, ok := graph.GetNode("node1")
	if !ok {
		t.Fatal("expected node to be retrieved")
	}

	if retrieved.Label != "Test Node" {
		t.Errorf("expected label 'Test Node', got '%s'", retrieved.Label)
	}

	if retrieved.Properties["key"] != "value" {
		t.Error("expected property 'key' to be 'value'")
	}
}

func TestKnowledgeGraph_AddNode_AutoID(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	node := &Node{
		Type:  "entity",
		Label: "Auto ID Node",
	}

	graph.AddNode(node)

	if node.ID == "" {
		t.Error("expected auto-generated ID")
	}

	if !node.CreatedAt.Before(time.Now().Add(time.Second)) {
		t.Error("expected CreatedAt to be set")
	}
}

func TestKnowledgeGraph_AddEdge(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	// 先添加节点
	graph.AddNode(&Node{ID: "node1", Type: "entity", Label: "Node 1"})
	graph.AddNode(&Node{ID: "node2", Type: "entity", Label: "Node 2"})

	// 添加边
	edge := &Edge{
		ID:     "edge1",
		Source: "node1",
		Target: "node2",
		Type:   "relates_to",
		Weight: 1.0,
	}

	graph.AddEdge(edge)

	// 通过检查出处添加了校验边缘
	if len(graph.outEdges["node1"]) == 0 {
		t.Error("expected edge to be added to outEdges")
	}
}

func TestKnowledgeGraph_GetNode(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	graph.AddNode(&Node{ID: "existing", Type: "entity", Label: "Existing"})

	// 测试已存在的节点
	node, ok := graph.GetNode("existing")
	if !ok || node == nil {
		t.Error("expected to find existing node")
	}

	// 测试不存在的节点
	node, ok = graph.GetNode("non-existing")
	if ok || node != nil {
		t.Error("expected not to find non-existing node")
	}
}

func TestKnowledgeGraph_GetNeighbors(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	// 创建一个简单的图表: A - > B - > C
	graph.AddNode(&Node{ID: "A", Type: "entity", Label: "Node A"})
	graph.AddNode(&Node{ID: "B", Type: "entity", Label: "Node B"})
	graph.AddNode(&Node{ID: "C", Type: "entity", Label: "Node C"})

	graph.AddEdge(&Edge{ID: "e1", Source: "A", Target: "B", Type: "connects"})
	graph.AddEdge(&Edge{ID: "e2", Source: "B", Target: "C", Type: "connects"})

	// 找A的邻居 深度1
	neighbors := graph.GetNeighbors("A", 1)
	if len(neighbors) != 1 {
		t.Errorf("expected 1 neighbor at depth 1, got %d", len(neighbors))
	}

	// 找到A的邻居,深度2
	neighbors = graph.GetNeighbors("A", 2)
	if len(neighbors) != 2 {
		t.Errorf("expected 2 neighbors at depth 2, got %d", len(neighbors))
	}
}

func TestKnowledgeGraph_GetNeighbors_Bidirectional(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	// 创建双向边缘的图表
	graph.AddNode(&Node{ID: "center", Type: "entity"})
	graph.AddNode(&Node{ID: "left", Type: "entity"})
	graph.AddNode(&Node{ID: "right", Type: "entity"})

	graph.AddEdge(&Edge{ID: "e1", Source: "left", Target: "center", Type: "connects"})
	graph.AddEdge(&Edge{ID: "e2", Source: "center", Target: "right", Type: "connects"})

	// 获取中心邻居( 应包括左右)
	neighbors := graph.GetNeighbors("center", 1)
	if len(neighbors) != 2 {
		t.Errorf("expected 2 neighbors, got %d", len(neighbors))
	}
}

func TestKnowledgeGraph_GetNeighbors_CycleDetection(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	// 创建周期: A- > B- > C- > A
	graph.AddNode(&Node{ID: "A", Type: "entity"})
	graph.AddNode(&Node{ID: "B", Type: "entity"})
	graph.AddNode(&Node{ID: "C", Type: "entity"})

	graph.AddEdge(&Edge{ID: "e1", Source: "A", Target: "B", Type: "connects"})
	graph.AddEdge(&Edge{ID: "e2", Source: "B", Target: "C", Type: "connects"})
	graph.AddEdge(&Edge{ID: "e3", Source: "C", Target: "A", Type: "connects"})

	// 不应由于循环检测而出现无限循环
	neighbors := graph.GetNeighbors("A", 10)

	// 应该找到除A本身以外的所有节点
	if len(neighbors) != 2 {
		t.Errorf("expected 2 neighbors (B and C), got %d", len(neighbors))
	}
}

func TestKnowledgeGraph_QueryByType(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	graph.AddNode(&Node{ID: "doc1", Type: "document", Label: "Doc 1"})
	graph.AddNode(&Node{ID: "doc2", Type: "document", Label: "Doc 2"})
	graph.AddNode(&Node{ID: "entity1", Type: "entity", Label: "Entity 1"})

	// 查询文档
	docs := graph.QueryByType("document")
	if len(docs) != 2 {
		t.Errorf("expected 2 documents, got %d", len(docs))
	}

	// 查询实体
	entities := graph.QueryByType("entity")
	if len(entities) != 1 {
		t.Errorf("expected 1 entity, got %d", len(entities))
	}

	// 查询不存在的类型
	others := graph.QueryByType("other")
	if len(others) != 0 {
		t.Errorf("expected 0 results for non-existing type, got %d", len(others))
	}
}

func TestKnowledgeGraph_Concurrent(t *testing.T) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	// 同时写
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			nodeID := "node" + string(rune('0'+id))
			graph.AddNode(&Node{ID: nodeID, Type: "entity"})
			done <- true
		}(i)
	}

	// 等待所有的去常规
	for i := 0; i < 10; i++ {
		<-done
	}

	// 计数节点
	count := 0
	for i := 0; i < 10; i++ {
		nodeID := "node" + string(rune('0'+i))
		if _, ok := graph.GetNode(nodeID); ok {
			count++
		}
	}

	if count != 10 {
		t.Errorf("expected 10 nodes after concurrent writes, got %d", count)
	}
}

func TestDefaultGraphRAGConfig(t *testing.T) {
	config := DefaultGraphRAGConfig()

	if config.MaxResults == 0 {
		t.Error("expected MaxResults to be set")
	}

	if config.VectorWeight == 0 {
		t.Error("expected VectorWeight to be set")
	}

	if config.GraphWeight == 0 {
		t.Error("expected GraphWeight to be set")
	}
}

func BenchmarkKnowledgeGraph_AddNode(b *testing.B) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph.AddNode(&Node{
			ID:    "node" + string(rune(i)),
			Type:  "entity",
			Label: "Test Node",
		})
	}
}

func BenchmarkKnowledgeGraph_GetNeighbors(b *testing.B) {
	logger := zap.NewNop()
	graph := NewKnowledgeGraph(logger)

	// 创建带有100个节点的图表
	for i := 0; i < 100; i++ {
		graph.AddNode(&Node{ID: "node" + string(rune(i)), Type: "entity"})
	}

	// 创建边缘
	for i := 0; i < 99; i++ {
		graph.AddEdge(&Edge{
			ID:     "edge" + string(rune(i)),
			Source: "node" + string(rune(i)),
			Target: "node" + string(rune(i+1)),
			Type:   "connects",
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		graph.GetNeighbors("node0", 3)
	}
}

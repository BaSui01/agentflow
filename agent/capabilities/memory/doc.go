// Package memory implements the multi-layer memory subsystem for agents.
//
// It provides:
//   - Short-term and working memory stores (InMemoryMemoryStore)
//   - Episodic memory (EpisodicStore, InMemoryEpisodicStore)
//   - Semantic memory / knowledge graph (KnowledgeGraph, InMemoryKnowledgeGraph)
//   - Observation pipeline (in the observation/ subpackage)
//   - Enhanced memory system (EnhancedMemorySystem) that unifies all layers
//   - Memory coordinator (Coordinator) for caching and recent-message management
//   - Memory runtime (MemoryRuntime) for policy-driven memory access
package memory

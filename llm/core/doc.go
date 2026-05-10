// Package core defines the LLM provider abstractions and contracts.
//
// This package sits at Layer 1 of the architecture. It defines the interfaces
// (Provider, ChatProvider, EmbeddingProvider, etc.) that all LLM provider
// implementations must satisfy. Concrete implementations live in llm/providers/.
//
// This package must not depend on agent/, rag/, workflow/, api/, cmd/, or internal/.
package core

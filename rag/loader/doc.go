// Package loader provides a unified DocumentLoader interface and common file loaders
// for the RAG pipeline.
//
// It bridges the gap between raw data sources (files, URLs, APIs) and the rag.Document
// type used by chunkers, retrievers, and vector stores. Each loader reads a specific
// format and produces []rag.Document with appropriate metadata.
//
// Supported formats out of the box:
//   - Plain text (.txt)
//   - Markdown (.md)
//   - CSV (.csv)
//   - JSON / JSONL (.json, .jsonl)
//
// Use LoaderRegistry to route loading by file extension:
//
//	registry := loader.NewLoaderRegistry()
//	docs, err := registry.Load(ctx, "/path/to/data.csv")
//
// Custom loaders can be registered for any extension:
//
//	registry.Register(".xml", myXMLLoader)
package loader

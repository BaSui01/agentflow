// Package config handles application configuration loading, validation, and hot-reload.
//
// It supports loading from YAML files, environment variables, and programmatic
// overrides. Configuration is structured into sections (Server, LLM, Database,
// RAG, Telemetry, etc.) with sensible defaults defined in defaults.go.
//
// This package sits in the infrastructure layer and may be imported by
// internal/app/bootstrap (composition root) and cmd/ (entry points).
// Domain packages (agent/, rag/, llm/) must not import config directly.
package config

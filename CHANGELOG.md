# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of AgentFlow framework
- Unified LLM abstraction layer with Provider interface
- Enterprise-grade resilience capabilities (retry, idempotency, circuit breaker)
- Native tool calling with ReAct loop implementation
- Streaming response support
- Intelligent context management with multiple pruning strategies
- Router and load balancing for multiple providers
- Complete observability with Prometheus metrics
- Agent base framework with state machine and memory management
- OpenAI and Claude provider implementations
- Comprehensive test suite with 100+ tests
- Example code for common use cases
- **Fully extensible Agent type system** - Users can define any custom Agent types

### Changed
- **BREAKING**: Replaced business-specific Agent types with generic, extensible types
  - Removed: TypeWriter, TypePlanner, TypeFormatter, TypeResearcher, TypePromptCurator, TypeNextOutline, TypeWorldBuilder, TypeOrchestrator
  - Added: TypeGeneric, TypeAssistant, TypeAnalyzer, TypeTranslator, TypeSummarizer, TypeReviewer
  - **Impact**: Users can now define any custom Agent types without framework limitations
  - **Migration**: Simply define your own Agent types as constants or strings

### Deprecated
- N/A

### Removed
- N/A

### Fixed
- N/A

### Security
- N/A

## [0.1.0] - 2026-01-25

### Added
- Initial extraction from AgentFlowCreativeHub project
- Core LLM abstraction layer
- Provider implementations (OpenAI, Claude)
- Agent framework
- Resilience capabilities
- Context management
- Tool calling system
- Observability and metrics
- Comprehensive documentation

[Unreleased]: https://github.com/yourusername/agentflow/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/yourusername/agentflow/releases/tag/v0.1.0

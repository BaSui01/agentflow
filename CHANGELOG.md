# Changelog

All notable changes to AgentFlow will be documented in this file.

Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
versioning follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0] - 2026-03-06

### Added
- Web Search provider with database persistence and auto-registration at startup
- Tool registration API with DB-driven auto-reload and shared registry
- OpenAI-compatible endpoints (`POST /v1/chat/completions`, `POST /v1/responses`)
- Gemini and Anthropic compatible endpoint tolerant parsing
- Livecheck regression tests for compatible endpoints
- LLM provider capability matrix documentation (9 capability columns + standalone providers)
- HTTP API overview in Chinese and English README

### Changed
- Unified chat and agent route parameters with multi-provider routing entry
- Replaced simplified implementations with full business implementations in agent, RAG, and audit modules
- Updated Chinese/English documentation to match actual codebase features
- Fixed directory navigation doc (removed references to non-existent directories)
- Upgraded architecture guard with stricter enforcement policies

### Fixed
- Architecture guard false positives
- CI pipeline: removed reference to non-existent `tests/contracts` package
- Makefile: fixed `test-e2e` and `test-integration` targets to use build tags instead of non-existent directories
- Livecheck script: replaced `panic()` with proper error handling

## [1.4.5] - 2026-03-02

### Added
- Unified multi-module protocol and capability implementation
- Tool registration routes and runtime reload regression tests
- Shared chat-agent tool registry

### Changed
- Converged processing pipeline docs and examples
- Cleaned up runtime assembly details

## [1.4.0] - 2026-02-28

### Added
- Multi-provider routing main entry
- Unified chat and agent route parameters
- Processing pipeline convergence

### Changed
- Architecture guard tooling improvements
- Cross-layer observability contract updates

---

For full commit history, see [GitHub Commits](https://github.com/BaSui01/agentflow/commits/master).

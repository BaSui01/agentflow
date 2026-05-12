// Package tools exposes the public facade for AgentFlow tool capability plumbing.
//
// The root package keeps stable user-facing constructors, interfaces, type aliases,
// and thin adapters while implementation details are split by responsibility into
// focused subpackages. New shared logic should live in the owning subpackage first;
// root files should delegate to those helpers rather than reintroducing a god package.
//
// # 子包职责与依赖图
//
// Current internal ownership is:
//
//   - registry/: capability indexes, panic recovery helpers, registry-oriented lookup
//     primitives, and registry health/query support.
//   - discovery/: matching, candidate selection, skill descriptors, skill search,
//     discovery filtering, and discovery-facing DTO conversion.
//   - execution/: tool input preparation, execution levels, composition ordering,
//     dependency checks, timeout/concurrency execution helpers.
//   - remote/: remote tool transport, HTTP/MCP/A2A/stdin transport normalization,
//     discovery protocol URL/query helpers.
//   - store/: storage primitives used by the tools facade.
//
// Dependency direction for the tools subtree is intentionally narrow:
//
//   - tools facade -> registry/, discovery/, execution/, remote/, store/
//   - registry/ -> no execution dependency
//   - discovery/ -> no execution dependency
//   - store/ -> no execution dependency
//   - remote/ owns protocol/transport helpers and does not import the tools facade
//
// Keep public API compatibility at the tools facade boundary. Internal helpers should
// move toward the owning subpackage, with root package functions reduced to adapters
// that translate existing public types into subpackage-owned contracts.
package tools

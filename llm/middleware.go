package llm

// =============================================================================
// LLM Middleware — Deprecated Shim
// =============================================================================
// All middleware types, interfaces, and functions have been consolidated into
// the llm/middleware sub-package (github.com/BaSui01/agentflow/llm/middleware).
//
// Previously this file contained duplicate definitions of:
//   - Handler, Middleware, Chain, NewChain
//   - LoggingMiddleware, TimeoutMiddleware, RecoveryMiddleware, MetricsMiddleware
//   - MetricsCollector interface
//   - PanicError struct
//
// These were 100% duplicated with llm/middleware/chain.go. The sub-package is
// the canonical location and contains additional middleware not present here
// (RetryMiddleware, CacheMiddleware, HeadersMiddleware, RateLimitMiddleware,
// TracingMiddleware, ValidatorMiddleware, TransformMiddleware).
//
// Import path: github.com/BaSui01/agentflow/llm/middleware
// =============================================================================

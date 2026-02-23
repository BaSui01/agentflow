package main

// Thin wrapper: re-exports from pkg/middleware for backward compatibility.
// New code should import "github.com/BaSui01/agentflow/pkg/middleware" directly.

import "github.com/BaSui01/agentflow/pkg/middleware"

// Type aliases for backward compatibility within cmd/agentflow.
type Middleware = middleware.Middleware

// Function aliases
var (
	Chain             = middleware.Chain
	Recovery          = middleware.Recovery
	RequestLogger     = middleware.RequestLogger
	MetricsMiddleware = middleware.MetricsMiddleware
	OTelTracing       = middleware.OTelTracing
	APIKeyAuth        = middleware.APIKeyAuth
	RateLimiter       = middleware.RateLimiter
	CORS              = middleware.CORS
	RequestID         = middleware.RequestID
	SecurityHeaders   = middleware.SecurityHeaders
	JWTAuth           = middleware.JWTAuth
	TenantRateLimiter = middleware.TenantRateLimiter
)

// RequestIDFromContext re-exports the context helper.
var RequestIDFromContext = middleware.RequestIDFromContext

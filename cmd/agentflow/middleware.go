package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/metrics"
	"github.com/BaSui01/agentflow/types"

	"github.com/golang-jwt/jwt/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// requestIDKey is the context key for the request ID.
type requestIDKey struct{}

// RequestIDFromContext extracts the request ID from the context.
// Returns an empty string if no request ID is present.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// Middleware 类型定义
type Middleware func(http.Handler) http.Handler

// Chain 将多个中间件串联
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// Recovery panic 恢复中间件
func Recovery(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("panic recovered", zap.Any("error", err), zap.String("path", r.URL.Path))
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger 请求日志中间件
func RequestLogger(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r)
			logger.Info("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.statusCode),
				zap.Duration("duration", time.Since(start)),
				zap.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// =============================================================================
// MetricsMiddleware — records HTTP request metrics via metrics.Collector
// =============================================================================

// metricsResponseWriter wraps http.ResponseWriter to capture status code and
// response body size for metrics recording.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
	bytesWritten int64
}

func (w *metricsResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.statusCode = code
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

// Flush implements http.Flusher for SSE streaming support.
func (w *metricsResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// MetricsMiddleware records HTTP request duration, status, and sizes via the
// provided metrics.Collector. Path labels are normalized to avoid high-cardinality
// Prometheus time series (e.g. "/api/v1/agents/abc123" becomes "/api/v1/agents/:id").
func MetricsMiddleware(collector *metrics.Collector) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			mrw := &metricsResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			next.ServeHTTP(mrw, r)

			duration := time.Since(start)
			path := normalizePath(r.URL.Path)
			requestSize := r.ContentLength
			if requestSize < 0 {
				requestSize = 0
			}

			collector.RecordHTTPRequest(
				r.Method,
				path,
				mrw.statusCode,
				duration,
				requestSize,
				mrw.bytesWritten,
			)
		})
	}
}

// pathSegmentPattern matches path segments that look like dynamic identifiers:
// UUIDs, hex strings (8+ chars), or numeric IDs.
var pathSegmentPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8,}(-[0-9a-fA-F]{4,}){0,4}$|^[0-9]+$`,
)

// normalizePath replaces dynamic path segments with ":id" to keep Prometheus
// label cardinality bounded. For example:
//
//	/api/v1/agents/abc123  -> /api/v1/agents/:id
//	/api/v1/chat/completions -> /api/v1/chat/completions (unchanged)
func normalizePath(path string) string {
	// Fast path for known static routes
	switch path {
	case "/health", "/healthz", "/ready", "/readyz", "/version", "/metrics",
		"/api/v1/chat/completions", "/api/v1/chat/completions/stream",
		"/api/v1/config", "/api/v1/config/reload",
		"/api/v1/config/fields", "/api/v1/config/changes":
		return path
	}

	segments := strings.Split(path, "/")
	normalized := false
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		if pathSegmentPattern.MatchString(seg) {
			segments[i] = ":id"
			normalized = true
		}
	}
	if !normalized {
		return path
	}
	return strings.Join(segments, "/")
}

// =============================================================================
// OTelTracing — OpenTelemetry HTTP tracing middleware
// =============================================================================

// OTelTracing creates a span for each HTTP request using the global OTel tracer.
// It extracts incoming trace context from request headers and records standard
// HTTP semantic convention attributes on the span.
func OTelTracing() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from incoming request headers
			propagator := otel.GetTextMapPropagator()
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			tracer := otel.Tracer("agentflow/http")
			spanName := r.Method + " " + r.URL.Path
			ctx, span := tracer.Start(ctx, spanName,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					semconv.HTTPRequestMethodKey.String(r.Method),
					semconv.URLFull(r.URL.String()),
				),
			)
			defer span.End()

			// Wrap response writer to capture status code
			rw := handlers.NewResponseWriter(w)
			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetAttributes(
				attribute.Int("http.response.status_code", rw.StatusCode),
			)
		})
	}
}

// APIKeyAuth API Key 认证中间件
// skipPaths 中的路径不需要认证（如 /health, /healthz, /ready, /readyz, /version, /metrics）
func APIKeyAuth(validKeys []string, skipPaths []string, allowQueryAPIKey bool, logger *zap.Logger) Middleware {
	keySet := make(map[string]struct{}, len(validKeys))
	for _, k := range validKeys {
		keySet[k] = struct{}{}
	}
	skipSet := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skipSet[p] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, skip := skipSet[r.URL.Path]; skip {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("X-API-Key")
			if allowQueryAPIKey && key == "" {
				key = r.URL.Query().Get("api_key")
			}
			if _, ok := keySet[key]; !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"error":"unauthorized","message":"invalid or missing API key"}`)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter 基于 IP 的请求限流中间件
func RateLimiter(ctx context.Context, rps float64, burst int, logger *zap.Logger) Middleware {
	type visitor struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mu       sync.Mutex
		visitors = make(map[string]*visitor)
	)
	// 后台清理过期 visitor
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				for ip, v := range visitors {
					if time.Since(v.lastSeen) > 3*time.Minute {
						delete(visitors, ip)
					}
				}
				mu.Unlock()
			}
		}
	}()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			mu.Lock()
			v, exists := visitors[ip]
			if !exists {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
				visitors[ip] = v
			}
			v.lastSeen = time.Now()
			mu.Unlock()
			if !v.limiter.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, `{"error":"rate_limit_exceeded","message":"too many requests"}`)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS 跨域中间件
// 安全修复：当 allowedOrigins 为空时，不设置 CORS 头（拒绝跨域请求），
// 而非默认允许所有来源（Access-Control-Allow-Origin: *）。
func CORS(allowedOrigins []string) Middleware {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if len(originSet) == 0 {
				// allowedOrigins 未配置：不设置任何 CORS 头，拒绝跨域请求
				// 生产环境应显式配置允许的来源
				if origin != "" {
					// 有 Origin 头的跨域请求，不设置 Allow-Origin，浏览器会拒绝
					if r.Method == http.MethodOptions {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					next.ServeHTTP(w, r)
					return
				}
			} else if _, ok := originSet[origin]; ok {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequestID adds a unique request ID to each request via the X-Request-ID header
// and injects it into the request context. If the client already provides one,
// it is preserved. Downstream handlers can retrieve the ID via RequestIDFromContext.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = generateRequestID()
			}
			w.Header().Set("X-Request-ID", id)
			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SecurityHeaders adds common security response headers to every request.
func SecurityHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			next.ServeHTTP(w, r)
		})
	}
}

// generateRequestID produces a random hex string suitable for request tracing.
func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "req-" + hex.EncodeToString(b)
}

// =============================================================================
// JWTAuth — JWT Bearer token authentication middleware
// =============================================================================

// JWTAuth validates JWT tokens from the Authorization: Bearer header and injects
// tenant_id, user_id, and roles into the request context using types.WithTenantID,
// types.WithUserID, and types.WithRoles. Supports HMAC (HS256) and RSA (RS256).
// skipPaths are exempt from authentication (e.g. health endpoints).
func JWTAuth(cfg config.JWTConfig, skipPaths []string, logger *zap.Logger) Middleware {
	skipSet := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skipSet[p] = struct{}{}
	}

	// Parse RSA public key if provided
	var rsaKey *rsa.PublicKey
	if cfg.PublicKey != "" {
		block, _ := pem.Decode([]byte(cfg.PublicKey))
		if block != nil {
			pub, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err == nil {
				if k, ok := pub.(*rsa.PublicKey); ok {
					rsaKey = k
				}
			}
			if rsaKey == nil {
				logger.Warn("failed to parse RSA public key, RSA verification disabled")
			}
		} else {
			logger.Warn("failed to decode PEM block for RSA public key")
		}
	}

	hmacSecret := []byte(cfg.Secret)

	// Build parser options
	parserOpts := []jwt.ParserOption{jwt.WithValidMethods([]string{"HS256", "RS256"})}
	if cfg.Issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(cfg.Issuer))
	}
	if cfg.Audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(cfg.Audience))
	}

	keyFunc := func(token *jwt.Token) (any, error) {
		switch token.Method.Alg() {
		case "HS256":
			if len(hmacSecret) == 0 {
				return nil, fmt.Errorf("HMAC secret not configured")
			}
			return hmacSecret, nil
		case "RS256":
			if rsaKey == nil {
				return nil, fmt.Errorf("RSA public key not configured")
			}
			return rsaKey, nil
		default:
			return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, skip := skipSet[r.URL.Path]; skip {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeJSONError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			token, err := jwt.Parse(tokenStr, keyFunc, parserOpts...)
			if err != nil {
				logger.Debug("JWT validation failed", zap.Error(err))
				writeJSONError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok || !token.Valid {
				writeJSONError(w, http.StatusUnauthorized, "invalid token claims")
				return
			}

			// Extract identity claims and inject into context
			ctx := r.Context()
			if tenantID, ok := claims["tenant_id"].(string); ok && tenantID != "" {
				ctx = types.WithTenantID(ctx, tenantID)
			}
			if userID, ok := claims["user_id"].(string); ok && userID != "" {
				ctx = types.WithUserID(ctx, userID)
			}
			if rolesRaw, ok := claims["roles"].([]any); ok {
				roles := make([]string, 0, len(rolesRaw))
				for _, r := range rolesRaw {
					if s, ok := r.(string); ok {
						roles = append(roles, s)
					}
				}
				if len(roles) > 0 {
					ctx = types.WithRoles(ctx, roles)
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeJSONError writes a JSON error response with the given status code and message.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"success":false,"error":{"code":"AUTHENTICATION","message":%q}}`, message)
}

// =============================================================================
// TenantRateLimiter — per-tenant rate limiting middleware
// =============================================================================

// TenantRateLimiter applies rate limiting based on the tenant_id in the request
// context (set by JWTAuth). If no tenant_id is present, it falls back to per-IP
// rate limiting. This ensures fair resource allocation across tenants.
func TenantRateLimiter(ctx context.Context, rps float64, burst int, logger *zap.Logger) Middleware {
	type visitor struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mu       sync.Mutex
		visitors = make(map[string]*visitor)
	)
	// Background cleanup of expired visitors
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				for key, v := range visitors {
					if time.Since(v.lastSeen) > 3*time.Minute {
						delete(visitors, key)
					}
				}
				mu.Unlock()
			}
		}
	}()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Determine rate limit key: tenant_id from context, fallback to IP
			key := ""
			if tenantID, ok := types.TenantID(r.Context()); ok {
				key = "tenant:" + tenantID
			} else {
				ip, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					ip = r.RemoteAddr
				}
				key = "ip:" + ip
			}

			mu.Lock()
			v, exists := visitors[key]
			if !exists {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
				visitors[key] = v
			}
			v.lastSeen = time.Now()
			mu.Unlock()

			if !v.limiter.Allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, `{"success":false,"error":{"code":"RATE_LIMITED","message":"tenant rate limit exceeded"}}`)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

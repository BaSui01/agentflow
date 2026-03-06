package middleware

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/pkg/metrics"
	"github.com/BaSui01/agentflow/pkg/telemetry"
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

const (
	visitorCleanupInterval = 3 * time.Minute
	minJWTSecretLength     = 32
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
					logger.Error("panic recovered",
						zap.Any("error", err),
						zap.Error(recoveredPanicToError(err)),
						zap.String("path", r.URL.Path),
						zap.Stack("stack"),
					)
					writeMiddlewareError(w, http.StatusInternalServerError, string(types.ErrInternalError), "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func recoveredPanicToError(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", v)
}

// RequestLogger 请求日志中间件
func RequestLogger(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(rw, r)
			traceLogger := telemetry.LoggerWithTrace(r.Context(), logger)
			traceLogger.Info("request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.statusCode),
				zap.Duration("duration", time.Since(start)),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("request_id", RequestIDFromContext(r.Context())),
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

// Flush implements http.Flusher for SSE streaming support.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker so WebSocket upgrades work through the logging middleware.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// =============================================================================
// MetricsMiddleware — records HTTP request metrics via metrics.Collector
// =============================================================================

// metricsResponseWriter wraps http.ResponseWriter to capture status code and
// response body size for metrics recording.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	wroteHeader  bool
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

// Hijack implements http.Hijacker so WebSocket upgrades work through the metrics middleware.
func (w *metricsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

type tracingResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func newTracingResponseWriter(w http.ResponseWriter) *tracingResponseWriter {
	return &tracingResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (w *tracingResponseWriter) WriteHeader(code int) {
	if w.written {
		return
	}
	w.statusCode = code
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *tracingResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for SSE streaming support.
func (w *tracingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *tracingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

// MetricsMiddleware records HTTP request duration, status, and sizes via the
// provided metrics.Collector.
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

var pathSegmentPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8,}(-[0-9a-fA-F]{4,}){0,4}$|^[0-9]+$`,
)

func normalizePath(path string) string {
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

func clientIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}

	remoteIP := parseIPFromAddr(r.RemoteAddr)
	if isTrustedForwardSource(remoteIP) {
		if ip := parseForwardedForHeader(r.Header.Get("X-Forwarded-For")); ip != "" {
			return ip
		}
		if ip := parseIPFromAddr(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
	}

	if remoteIP != "" {
		return remoteIP
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func parseForwardedForHeader(value string) string {
	for _, part := range strings.Split(value, ",") {
		if ip := parseIPFromAddr(part); ip != "" {
			return ip
		}
	}
	return ""
}

func parseIPFromAddr(raw string) string {
	trimmed := strings.TrimSpace(strings.Trim(raw, "[]"))
	if trimmed == "" {
		return ""
	}
	if ip := net.ParseIP(trimmed); ip != nil {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(trimmed)
	if err != nil {
		return ""
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return ""
}

func isTrustedForwardSource(ipStr string) bool {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// OTelTracing creates a span for each HTTP request using the global OTel tracer.
func OTelTracing() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			ctx = types.WithTraceID(ctx, span.SpanContext().TraceID().String())

			rw := newTracingResponseWriter(w)
			next.ServeHTTP(rw, r.WithContext(ctx))

			span.SetAttributes(
				attribute.Int("http.response.status_code", rw.statusCode),
			)
		})
	}
}

// APIKeyAuth API Key 认证中间件（仅支持 X-API-Key header）
func APIKeyAuth(validKeys []string, skipPaths []string, logger *zap.Logger) Middleware {
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
			if _, ok := keySet[key]; !ok {
				logger.Debug("API key auth failed", zap.String("path", r.URL.Path))
				writeMiddlewareError(w, http.StatusUnauthorized, string(types.ErrAuthentication), "invalid or missing API key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimiter 基于 IP 的请求限流中间件
func RateLimiter(ctx context.Context, rps float64, burst int, logger *zap.Logger) Middleware {
	if ctx == nil {
		ctx = context.Background()
	}
	type visitor struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mu          sync.Mutex
		visitors    = make(map[string]*visitor)
		lastCleanup = time.Now()
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-ctx.Done():
				if logger != nil {
					logger.Warn("rate limiter context canceled", zap.Error(ctx.Err()))
				}
				writeMiddlewareError(w, http.StatusServiceUnavailable, string(types.ErrServiceUnavailable), "service shutting down")
				return
			default:
			}

			now := time.Now()
			ip := clientIPFromRequest(r)
			mu.Lock()
			if now.Sub(lastCleanup) >= time.Minute {
				for k, v := range visitors {
					if now.Sub(v.lastSeen) > visitorCleanupInterval {
						delete(visitors, k)
					}
				}
				lastCleanup = now
			}
			v, exists := visitors[ip]
			if !exists {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
				visitors[ip] = v
			}
			v.lastSeen = now
			mu.Unlock()
			if !v.limiter.Allow() {
				writeMiddlewareError(w, http.StatusTooManyRequests, string(types.ErrRateLimit), "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS 跨域中间件
func CORS(allowedOrigins []string) Middleware {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if len(originSet) == 0 {
				if origin != "" {
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

// RequestID adds a unique request ID to each request.
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

// SecurityHeaders adds common security response headers.
func SecurityHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			next.ServeHTTP(w, r)
		})
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// time-based fallback when crypto/rand fails
		return fmt.Sprintf("req-%x", time.Now().UnixNano())
	}
	return "req-" + hex.EncodeToString(b)
}

// JWTAuth validates JWT tokens from the Authorization: Bearer header and injects
// tenant_id, user_id, and roles into the request context.
// Returns error when HMAC secret is configured but shorter than minJWTSecretLength (reject startup).
func JWTAuth(cfg config.JWTConfig, skipPaths []string, logger *zap.Logger) (Middleware, error) {
	if len(cfg.Secret) > 0 && len(cfg.Secret) < minJWTSecretLength {
		logger.Warn("JWT HMAC secret is shorter than 32 bytes — NOT recommended for production",
			zap.Int("length", len(cfg.Secret)),
		)
		return nil, fmt.Errorf("JWT HMAC secret must be at least %d bytes, got %d", minJWTSecretLength, len(cfg.Secret))
	}

	skipSet := make(map[string]struct{}, len(skipPaths))
	for _, p := range skipPaths {
		skipSet[p] = struct{}{}
	}

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

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256", "RS256"}),
		jwt.WithExpirationRequired(),
	}
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
				writeMiddlewareError(w, http.StatusUnauthorized, string(types.ErrAuthentication), "missing or malformed Authorization header")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			token, err := jwt.Parse(tokenStr, keyFunc, parserOpts...)
			if err != nil {
				logger.Debug("JWT validation failed", zap.Error(err))
				writeMiddlewareError(w, http.StatusUnauthorized, string(types.ErrAuthentication), "invalid or expired token")
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok || !token.Valid {
				writeMiddlewareError(w, http.StatusUnauthorized, string(types.ErrAuthentication), "invalid token claims")
				return
			}

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
	}, nil
}

// writeMiddlewareError writes a JSON error response.
func writeMiddlewareError(w http.ResponseWriter, statusCode int, code string, message string) {
	resp := middlewareErrorResponse{
		Success: false,
		Error: &middlewareErrorInfo{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now().UTC(),
	}

	buf, err := json.Marshal(resp)
	if err != nil {
		fallback := middlewareErrorResponse{
			Success: false,
			Error: &middlewareErrorInfo{
				Code:    string(types.ErrInternalError),
				Message: "failed to encode response",
			},
			Timestamp: time.Now().UTC(),
		}
		buf, err = json.Marshal(fallback)
		if err != nil {
			log.Printf("[middleware] json.Marshal(fallback) failed: %v", err)
			buf = []byte(`{"success":false,"error":{"code":"INTERNAL_ERROR","message":"failed to encode response"}}`)
		}
		statusCode = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	_, _ = w.Write(buf)
}

type middlewareErrorResponse struct {
	Success   bool                 `json:"success"`
	Error     *middlewareErrorInfo `json:"error,omitempty"`
	Timestamp time.Time            `json:"timestamp"`
}

type middlewareErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TenantRateLimiter applies rate limiting based on the tenant_id in the request context.
func TenantRateLimiter(ctx context.Context, rps float64, burst int, logger *zap.Logger) Middleware {
	if ctx == nil {
		ctx = context.Background()
	}
	type visitor struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mu          sync.Mutex
		visitors    = make(map[string]*visitor)
		lastCleanup = time.Now()
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-ctx.Done():
				if logger != nil {
					logger.Warn("tenant rate limiter context canceled", zap.Error(ctx.Err()))
				}
				writeMiddlewareError(w, http.StatusServiceUnavailable, string(types.ErrServiceUnavailable), "service shutting down")
				return
			default:
			}

			now := time.Now()
			key := ""
			if tenantID, ok := types.TenantID(r.Context()); ok {
				key = "tenant:" + tenantID
			} else {
				ip := clientIPFromRequest(r)
				key = "ip:" + ip
			}

			mu.Lock()
			if now.Sub(lastCleanup) >= time.Minute {
				for k, v := range visitors {
					if now.Sub(v.lastSeen) > visitorCleanupInterval {
						delete(visitors, k)
					}
				}
				lastCleanup = now
			}
			v, exists := visitors[key]
			if !exists {
				v = &visitor{limiter: rate.NewLimiter(rate.Limit(rps), burst)}
				visitors[key] = v
			}
			v.lastSeen = now
			mu.Unlock()

			if !v.limiter.Allow() {
				writeMiddlewareError(w, http.StatusTooManyRequests, string(types.ErrRateLimit), "tenant rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

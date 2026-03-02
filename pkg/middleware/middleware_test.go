package middleware

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/types"
)

type flushTrackingWriter struct {
	*httptest.ResponseRecorder
	flushed bool
}

func (w *flushTrackingWriter) Flush() {
	w.flushed = true
	w.ResponseRecorder.Flush()
}

func (w *flushTrackingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func encodeRSAPublicKeyPEM(pub *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

// --- Chain ---

func TestChain(t *testing.T) {
	var order []string
	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1")
			next.ServeHTTP(w, r)
		})
	}
	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2")
			next.ServeHTTP(w, r)
		})
	}

	handler := Chain(okHandler(), m1, m2)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	assert.Equal(t, []string{"m1", "m2"}, order)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- Recovery ---

func TestRecovery(t *testing.T) {
	logger := zap.NewNop()
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	handler := Recovery(logger)(panicHandler)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/panic", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, false, resp["success"])
}

func TestRecovery_NoPanic(t *testing.T) {
	handler := Recovery(zap.NewNop())(okHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/ok", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- RequestLogger ---

func TestRequestLogger(t *testing.T) {
	handler := RequestLogger(zap.NewNop())(okHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- responseWriter ---

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}
	rw.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, rw.statusCode)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestResponseWriter_Flush(t *testing.T) {
	rec := &flushTrackingWriter{ResponseRecorder: httptest.NewRecorder()}
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}
	rw.Flush()
	assert.True(t, rec.flushed)
}

// --- metricsResponseWriter ---

func TestMetricsResponseWriter_WriteHeader_OnlyOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	mrw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}
	mrw.WriteHeader(http.StatusCreated)
	mrw.WriteHeader(http.StatusNotFound) // should be ignored
	assert.Equal(t, http.StatusCreated, mrw.statusCode)
	assert.True(t, mrw.wroteHeader)
}

func TestMetricsResponseWriter_Write(t *testing.T) {
	rec := httptest.NewRecorder()
	mrw := &metricsResponseWriter{ResponseWriter: rec, statusCode: http.StatusOK}
	n, err := mrw.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, int64(5), mrw.bytesWritten)
	assert.True(t, mrw.wroteHeader) // auto-set on first Write
}

func TestMetricsResponseWriter_Flush(t *testing.T) {
	rec := httptest.NewRecorder()
	mrw := &metricsResponseWriter{ResponseWriter: rec}
	// Should not panic even if underlying doesn't implement Flusher
	mrw.Flush()
}

func TestTracingResponseWriter_Flush(t *testing.T) {
	rec := &flushTrackingWriter{ResponseRecorder: httptest.NewRecorder()}
	rw := newTracingResponseWriter(rec)
	rw.Flush()
	assert.True(t, rec.flushed)
}

// --- normalizePath ---

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/health", "/health"},
		{"/healthz", "/healthz"},
		{"/api/v1/chat/completions", "/api/v1/chat/completions"},
		{"/api/v1/users/12345", "/api/v1/users/:id"},
		{"/api/v1/users/550e8400-e29b-41d4-a716-446655440000", "/api/v1/users/:id"},
		{"/api/v1/users/abcdef12", "/api/v1/users/:id"},
		{"/api/v1/items/hello", "/api/v1/items/hello"}, // not numeric/hex
		{"/unknown/path", "/unknown/path"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizePath(tt.input))
		})
	}
}

// --- APIKeyAuth ---

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	handler := APIKeyAuth([]string{"valid-key"}, nil, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "valid-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	handler := APIKeyAuth([]string{"valid-key"}, nil, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyAuth_MissingKey(t *testing.T) {
	handler := APIKeyAuth([]string{"valid-key"}, nil, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyAuth_SkipPath(t *testing.T) {
	handler := APIKeyAuth([]string{"key"}, []string{"/health"}, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestAPIKeyAuth_QueryParam(t *testing.T) {
	handler := APIKeyAuth([]string{"query-key"}, nil, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/api/test?api_key=query-key", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyAuth_QueryParamDisabled(t *testing.T) {
	handler := APIKeyAuth([]string{"query-key"}, nil, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/api/test?api_key=query-key", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- CORS ---

func TestCORS_AllowedOrigin(t *testing.T) {
	handler := CORS([]string{"https://example.com"})(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "https://example.com", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_DisallowedOrigin(t *testing.T) {
	handler := CORS([]string{"https://example.com"})(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_Preflight(t *testing.T) {
	handler := CORS([]string{"https://example.com"})(okHandler())
	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCORS_EmptyOrigins_WithOriginHeader(t *testing.T) {
	handler := CORS(nil)(okHandler())
	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCORS_EmptyOrigins_NoOriginHeader(t *testing.T) {
	handler := CORS(nil)(okHandler())
	req := httptest.NewRequest("OPTIONS", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// --- RequestID ---

func TestRequestID_Generated(t *testing.T) {
	handler := RequestID()(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	id := rec.Header().Get("X-Request-ID")
	assert.True(t, strings.HasPrefix(id, "req-"))
	assert.Len(t, id, 4+32) // "req-" + 32 hex chars
}

func TestRequestID_Preserved(t *testing.T) {
	handler := RequestID()(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "custom-id-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, "custom-id-123", rec.Header().Get("X-Request-ID"))
}

func TestRequestIDFromContext(t *testing.T) {
	var gotID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = RequestIDFromContext(r.Context())
	})
	handler := RequestID()(inner)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "ctx-test-id")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, "ctx-test-id", gotID)
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	assert.Equal(t, "", RequestIDFromContext(context.Background()))
}

// --- SecurityHeaders ---

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders()(okHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
	assert.Equal(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
	assert.Equal(t, "default-src 'self'", rec.Header().Get("Content-Security-Policy"))
	assert.Contains(t, rec.Header().Get("Strict-Transport-Security"), "max-age=31536000")
}

// --- RateLimiter ---

func TestRateLimiter_AllowsRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := RateLimiter(ctx, 100, 10, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRateLimiter_BlocksExcessRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := RateLimiter(ctx, 1, 1, zap.NewNop())(okHandler())

	// First request should pass
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request should be rate limited
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestRateLimiter_UsesForwardedIPWhenProxyTrusted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := RateLimiter(ctx, 1, 1, zap.NewNop())(okHandler())

	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "10.0.0.1:1234"
	req1.Header.Set("X-Forwarded-For", "203.0.113.10")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "10.0.0.1:4321"
	req2.Header.Set("X-Forwarded-For", "203.0.113.11")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestRateLimiter_IgnoresForwardedIPWhenRemoteUntrusted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := RateLimiter(ctx, 1, 1, zap.NewNop())(okHandler())

	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "8.8.8.8:1234"
	req1.Header.Set("X-Forwarded-For", "203.0.113.10")
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "8.8.8.8:4321"
	req2.Header.Set("X-Forwarded-For", "203.0.113.11")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

// --- TenantRateLimiter ---

func TestTenantRateLimiter_WithTenantID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := TenantRateLimiter(ctx, 1, 1, zap.NewNop())(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	tenantCtx := types.WithTenantID(req.Context(), "tenant-1")
	req = req.WithContext(tenantCtx)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Second request from same tenant should be limited
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
}

func TestTenantRateLimiter_FallbackToIP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := TenantRateLimiter(ctx, 100, 10, zap.NewNop())(okHandler())

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "10.0.0.2:5678"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- JWTAuth ---

func TestJWTAuth_ValidHMACToken(t *testing.T) {
	secret := "this-is-a-very-long-secret-key-for-testing-purposes"
	cfg := config.JWTConfig{Secret: secret}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"tenant_id": "t-123",
		"user_id":   "u-456",
		"roles":     []string{"admin"},
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString([]byte(secret))
	require.NoError(t, err)

	var gotTenantID, gotUserID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tid, ok := types.TenantID(r.Context()); ok {
			gotTenantID = tid
		}
		if uid, ok := types.UserID(r.Context()); ok {
			gotUserID = uid
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := JWTAuth(cfg, nil, zap.NewNop())(inner)
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "t-123", gotTenantID)
	assert.Equal(t, "u-456", gotUserID)
}

func TestJWTAuth_MissingHeader(t *testing.T) {
	cfg := config.JWTConfig{Secret: "this-is-a-very-long-secret-key-for-testing-purposes"}
	handler := JWTAuth(cfg, nil, zap.NewNop())(okHandler())

	req := httptest.NewRequest("GET", "/api/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTAuth_InvalidToken(t *testing.T) {
	cfg := config.JWTConfig{Secret: "this-is-a-very-long-secret-key-for-testing-purposes"}
	handler := JWTAuth(cfg, nil, zap.NewNop())(okHandler())

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTAuth_SkipPath(t *testing.T) {
	cfg := config.JWTConfig{Secret: "this-is-a-very-long-secret-key-for-testing-purposes"}
	handler := JWTAuth(cfg, []string{"/health"}, zap.NewNop())(okHandler())

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJWTAuth_ExpiredToken(t *testing.T) {
	secret := "this-is-a-very-long-secret-key-for-testing-purposes"
	cfg := config.JWTConfig{Secret: secret}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(secret))

	handler := JWTAuth(cfg, nil, zap.NewNop())(okHandler())
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWTAuth_RSAToken(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// We need to encode the public key as PEM for the config
	pubKeyBytes, err := encodeRSAPublicKeyPEM(&privateKey.PublicKey)
	require.NoError(t, err)

	cfg := config.JWTConfig{PublicKey: string(pubKeyBytes)}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"tenant_id": "rsa-tenant",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, err := token.SignedString(privateKey)
	require.NoError(t, err)

	var gotTenantID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tid, ok := types.TenantID(r.Context()); ok {
			gotTenantID = tid
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := JWTAuth(cfg, nil, zap.NewNop())(inner)
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "rsa-tenant", gotTenantID)
}

// --- writeMiddlewareError ---

func TestWriteMiddlewareError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeMiddlewareError(rec, http.StatusForbidden, "FORBIDDEN", "access denied")

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	var resp map[string]any
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, false, resp["success"])
	errObj := resp["error"].(map[string]any)
	assert.Equal(t, "FORBIDDEN", errObj["code"])
	assert.Equal(t, "access denied", errObj["message"])
}

// --- generateRequestID ---

func TestGenerateRequestID(t *testing.T) {
	id := generateRequestID()
	assert.True(t, strings.HasPrefix(id, "req-"))
	assert.Len(t, id, 36) // "req-" + 32 hex chars

	// Ensure uniqueness
	id2 := generateRequestID()
	assert.NotEqual(t, id, id2)
}

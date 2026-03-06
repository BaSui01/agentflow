package bootstrap

import (
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/config"
	mw "github.com/BaSui01/agentflow/pkg/middleware"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// BuildAuthMiddleware selects and creates the HTTP auth middleware.
// Priority: JWT (if secret or public key configured) > API Key > fail-closed.
func BuildAuthMiddleware(serverCfg config.ServerConfig, skipPaths []string, logger *zap.Logger) (mw.Middleware, error) {
	jwtCfg := serverCfg.JWT
	hasJWT := jwtCfg.Secret != "" || jwtCfg.PublicKey != ""
	hasAPIKeys := len(serverCfg.APIKeys) > 0

	switch {
	case hasJWT:
		logger.Info("Authentication: JWT enabled",
			zap.Bool("hmac", jwtCfg.Secret != ""),
			zap.Bool("rsa", jwtCfg.PublicKey != ""),
			zap.String("issuer", jwtCfg.Issuer),
		)
		return mw.JWTAuth(jwtCfg, skipPaths, logger)
	case hasAPIKeys:
		logger.Info("Authentication: API Key enabled",
			zap.Int("key_count", len(serverCfg.APIKeys)),
		)
		return mw.APIKeyAuth(serverCfg.APIKeys, skipPaths, logger), nil
	default:
		if serverCfg.AllowNoAuth {
			logger.Warn("Authentication is disabled (allow_no_auth=true). " +
				"This is not recommended for production use.")
			return nil, nil
		}
		logger.Error("Authentication is required but no JWT/API key is configured; protected endpoints will return 503 until fixed. Configure server.api_keys or server.jwt, or explicitly set server.allow_no_auth=true for local development. Use /ready for liveness and a protected endpoint smoke test to verify auth wiring.")
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
				api.WriteJSONResponse(w, http.StatusServiceUnavailable, api.Response{
					Success: false,
					Error: &api.ErrorInfo{
						Code:    string(types.ErrServiceUnavailable),
						Message: "authentication is not configured",
					},
					Timestamp: time.Now().UTC(),
					RequestID: w.Header().Get("X-Request-ID"),
				})
			})
		}, nil
	}
}

package handlers

import (
	"net/http"
	"strings"

	"go.uber.org/zap"
)

func toolAuditFields(r *http.Request, resource, action, result string, extra ...zap.Field) []zap.Field {
	fields := []zap.Field{
		zap.String("request_id", strings.TrimSpace(r.Header.Get("X-Request-ID"))),
		zap.String("remote_addr", strings.TrimSpace(r.RemoteAddr)),
		zap.String("path", toolRequestPath(r)),
		zap.String("method", strings.TrimSpace(r.Method)),
		zap.String("resource", resource),
		zap.String("action", action),
		zap.String("result", result),
	}
	fields = append(fields, extra...)
	return fields
}

func logToolRequestInfo(logger *zap.Logger, r *http.Request, resource, action, result, message string, extra ...zap.Field) {
	if logger == nil {
		return
	}
	logger.Info(message, toolAuditFields(r, resource, action, result, extra...)...)
}

func logToolRequestWarn(logger *zap.Logger, r *http.Request, resource, action, result, message string, extra ...zap.Field) {
	if logger == nil {
		return
	}
	logger.Warn(message, toolAuditFields(r, resource, action, result, extra...)...)
}

func toolRequestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return strings.TrimSpace(r.URL.Path)
}

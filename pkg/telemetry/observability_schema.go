package telemetry

import (
	"strings"

	"go.opentelemetry.io/otel/attribute"
)

const (
	LogFieldTraceID = "trace_id"
	LogFieldSpanID  = "span_id"
)

var (
	AttrTraceID   = attribute.Key("trace.id")
	AttrTraceType = attribute.Key("trace.type")

	AttrTenantID = attribute.Key("tenant.id")
	AttrUserID   = attribute.Key("user.id")

	AttrLLMProvider      = attribute.Key("llm.provider")
	AttrLLMModel         = attribute.Key("llm.model")
	AttrLLMFeature       = attribute.Key("llm.feature")
	AttrLLMStatus        = attribute.Key("llm.status")
	AttrLLMTokenType     = attribute.Key("llm.token.type")
	AttrLLMCacheHit      = attribute.Key("llm.cache.hit")
	AttrLLMFallback      = attribute.Key("llm.fallback")
	AttrLLMFallbackLevel = attribute.Key("llm.fallback.level")
	AttrLLMDurationMS    = attribute.Key("llm.duration.ms")
	AttrLLMCost          = attribute.Key("llm.cost")

	AttrToolName       = attribute.Key("tool.name")
	AttrToolSuccess    = attribute.Key("tool.success")
	AttrToolDurationMS = attribute.Key("tool.duration.ms")

	AttrErrorCode    = attribute.Key("error.code")
	AttrErrorMessage = attribute.Key("error.message")
)

func LLMIdentityAttrs(provider, model, tenantID, userID, feature string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 5)
	attrs = appendStringAttr(attrs, AttrLLMProvider, provider)
	attrs = appendStringAttr(attrs, AttrLLMModel, model)
	attrs = appendStringAttr(attrs, AttrTenantID, tenantID)
	attrs = appendStringAttr(attrs, AttrUserID, userID)
	attrs = appendStringAttr(attrs, AttrLLMFeature, feature)
	return attrs
}

func LLMRequestAttrs(provider, model, tenantID, userID, feature, status string) []attribute.KeyValue {
	return appendStringAttr(LLMIdentityAttrs(provider, model, tenantID, userID, feature), AttrLLMStatus, status)
}

func LLMTraceAttrs(provider, model, tenantID, userID, feature, traceID string) []attribute.KeyValue {
	return appendStringAttr(LLMIdentityAttrs(provider, model, tenantID, userID, feature), AttrTraceID, traceID)
}

func LLMTokenAttrs(provider, model, tenantID, userID, feature, tokenType string) []attribute.KeyValue {
	return appendStringAttr(LLMIdentityAttrs(provider, model, tenantID, userID, feature), AttrLLMTokenType, tokenType)
}

func appendStringAttr(attrs []attribute.KeyValue, key attribute.Key, value string) []attribute.KeyValue {
	if strings.TrimSpace(value) == "" {
		return attrs
	}
	return append(attrs, key.String(value))
}

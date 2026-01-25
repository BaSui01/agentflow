package llm

import (
	"context"
	"encoding/json"
)

type credentialOverrideKey struct{}

// CredentialOverride 用于在单次请求内覆盖 Provider 凭据。
// 注意：该结构仅通过 context 传递，不会从 API JSON 反序列化，避免前端直接注入敏感信息。
type CredentialOverride struct {
	APIKey    string
	SecretKey string
}

func (c CredentialOverride) String() string {
	if c.APIKey == "" && c.SecretKey == "" {
		return "CredentialOverride{}"
	}
	return "CredentialOverride{APIKey:***, SecretKey:***}"
}

func (c CredentialOverride) MarshalJSON() ([]byte, error) {
	type masked struct {
		APIKey    string `json:"api_key,omitempty"`
		SecretKey string `json:"secret_key,omitempty"`
	}
	out := masked{}
	if c.APIKey != "" {
		out.APIKey = "***"
	}
	if c.SecretKey != "" {
		out.SecretKey = "***"
	}
	return json.Marshal(out)
}

// WithCredentialOverride 在 ctx 中写入凭据覆盖信息。
// 传入空的 APIKey/SecretKey 不会改变 ctx。
func WithCredentialOverride(ctx context.Context, c CredentialOverride) context.Context {
	if c.APIKey == "" && c.SecretKey == "" {
		return ctx
	}
	return context.WithValue(ctx, credentialOverrideKey{}, c)
}

// CredentialOverrideFromContext 从 ctx 读取凭据覆盖信息。
func CredentialOverrideFromContext(ctx context.Context) (CredentialOverride, bool) {
	v := ctx.Value(credentialOverrideKey{})
	if v == nil {
		return CredentialOverride{}, false
	}
	c, ok := v.(CredentialOverride)
	return c, ok
}

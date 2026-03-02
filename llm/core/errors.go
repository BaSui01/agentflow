package core

import (
	"fmt"

	"github.com/BaSui01/agentflow/types"
)

// InvalidCapabilityError 表示不支持的能力类型。
func InvalidCapabilityError(cap Capability) *types.Error {
	return types.NewInvalidRequestError(fmt.Sprintf("unsupported capability: %s", cap))
}

// InvalidPayloadError 表示 payload 类型或内容错误。
func InvalidPayloadError(cap Capability, expected string) *types.Error {
	return types.NewInvalidRequestError(
		fmt.Sprintf("invalid payload for capability %s, expected %s", cap, expected),
	)
}

// GatewayUnavailableError 表示 gateway 缺少执行依赖。
func GatewayUnavailableError(msg string) *types.Error {
	return types.NewServiceUnavailableError(msg)
}

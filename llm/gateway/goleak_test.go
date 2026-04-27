package gateway

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreAnyFunction("github.com/BaSui01/agentflow/llm/gateway.(*boostSlowStreamProvider).Stream.func1"),
		goleak.IgnoreAnyFunction("github.com/BaSui01/agentflow/llm/gateway.TestChatProviderAdapter_Stream_ContextCancel.func1"),
	)
}

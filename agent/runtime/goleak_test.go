package runtime

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
		goleak.IgnoreAnyFunction("github.com/BaSui01/agentflow/agent/runtime.(*ManagedLSP).start"),
		goleak.IgnoreTopFunction("io.(*pipe).read"),
		goleak.IgnoreTopFunction("io.(*pipe).write"),
	)
}

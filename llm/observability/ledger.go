package observability

import (
	"context"
	"time"

	llmcore "github.com/BaSui01/agentflow/llm/core"
)

// Ledger 定义统一的 usage/cost 落账接口。
type Ledger interface {
	Record(ctx context.Context, entry LedgerEntry) error
}

// LedgerEntry 定义一次统一出口落账记录。
type LedgerEntry struct {
	Timestamp time.Time `json:"timestamp"`

	TraceID    string `json:"trace_id,omitempty"`
	Capability string `json:"capability,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	Strategy   string `json:"strategy,omitempty"`

	Usage llmcore.Usage `json:"usage"`
	Cost  llmcore.Cost  `json:"cost"`

	Metadata map[string]string `json:"metadata,omitempty"`
}

type noopLedger struct{}

// NewNoopLedger 返回默认的无副作用落账器。
func NewNoopLedger() Ledger {
	return noopLedger{}
}

func (noopLedger) Record(context.Context, LedgerEntry) error {
	return nil
}

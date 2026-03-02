package core

// Usage 统一记录 token 与能力单位消耗。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`

	InputUnits  int `json:"input_units,omitempty"`
	OutputUnits int `json:"output_units,omitempty"`
	TotalUnits  int `json:"total_units,omitempty"`
}

// IsZero 判断 usage 是否为空统计。
func (u Usage) IsZero() bool {
	return u.PromptTokens == 0 &&
		u.CompletionTokens == 0 &&
		u.TotalTokens == 0 &&
		u.InputUnits == 0 &&
		u.OutputUnits == 0 &&
		u.TotalUnits == 0
}

// Cost 统一记录成本信息。
type Cost struct {
	AmountUSD float64 `json:"amount_usd,omitempty"`
	Currency  string  `json:"currency,omitempty"`
}

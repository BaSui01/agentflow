package core

import "strings"

const (
	// MetadataKeyChatProvider is the canonical metadata key for chat provider hint.
	MetadataKeyChatProvider = "chat_provider"
)

// CapabilityHints carries normalized cross-capability routing hints.
type CapabilityHints struct {
	ChatProvider   string `json:"chat_provider,omitempty"`
	RerankProvider string `json:"rerank_provider,omitempty"`
}

// Normalize trims whitespace in all hint fields.
func (h *CapabilityHints) Normalize() {
	if h == nil {
		return
	}
	h.ChatProvider = strings.TrimSpace(h.ChatProvider)
	h.RerankProvider = strings.TrimSpace(h.RerankProvider)
}

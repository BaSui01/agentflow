package core

import (
	"errors"
	"testing"

	"github.com/BaSui01/agentflow/types"
)

func TestRAGErrorCodeMapping(t *testing.T) {
	base := errors.New("cause")
	tests := []struct {
		name string
		err  *RAGError
		code types.ErrorCode
	}{
		{"upstream", ErrUpstream("up", base), types.ErrUpstreamError},
		{"timeout", ErrTimeout("to", base), types.ErrTimeout},
		{"internal", ErrInternal("in", base), types.ErrInternalError},
		{"config", ErrConfig("cfg", base), types.ErrInvalidRequest},
	}
	for _, tt := range tests {
		if tt.err.Code != tt.code {
			t.Fatalf("%s: got %s want %s", tt.name, tt.err.Code, tt.code)
		}
		if !errors.Is(tt.err, base) {
			t.Fatalf("%s: expected wrapped cause", tt.name)
		}
	}
}

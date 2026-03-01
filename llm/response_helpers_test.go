package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFirstChoice(t *testing.T) {
	tests := []struct {
		name    string
		resp    *ChatResponse
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil response",
			resp:    nil,
			wantErr: true,
			errMsg:  "nil ChatResponse",
		},
		{
			name:    "empty choices",
			resp:    &ChatResponse{Choices: []ChatChoice{}},
			wantErr: true,
			errMsg:  "empty choices",
		},
		{
			name: "single choice",
			resp: &ChatResponse{
				Choices: []ChatChoice{
					{Index: 0, Message: Message{Content: "hello"}},
				},
			},
			wantErr: false,
		},
		{
			name: "multiple choices returns first",
			resp: &ChatResponse{
				Choices: []ChatChoice{
					{Index: 0, Message: Message{Content: "first"}},
					{Index: 1, Message: Message{Content: "second"}},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			choice, err := FirstChoice(tt.resp)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.resp.Choices[0], choice)
			}
		})
	}
}


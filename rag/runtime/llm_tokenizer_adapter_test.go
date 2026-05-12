package runtime

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

type sharedTokenizerStub struct {
	countErr  bool
	encodeErr bool
}

func (s sharedTokenizerStub) CountTokens(text string) (int, error) {
	if s.countErr {
		return 0, errors.New("count failed")
	}
	return len(text), nil
}

func (s sharedTokenizerStub) Encode(text string) ([]int, error) {
	if s.encodeErr {
		return nil, errors.New("encode failed")
	}
	return []int{len(text)}, nil
}

func (s sharedTokenizerStub) Decode([]int) (string, error) { return "", nil }
func (s sharedTokenizerStub) MaxTokens() int               { return 4096 }
func (s sharedTokenizerStub) Name() string                 { return "shared-stub" }

func TestNewSharedTokenizerAdapterAdaptsRAGTokenizer(t *testing.T) {
	adapter := NewSharedTokenizerAdapter(sharedTokenizerStub{}, zap.NewNop())

	assert.Equal(t, 5, adapter.CountTokens("hello"))
	assert.Equal(t, []int{5}, adapter.Encode("hello"))
}

func TestSharedTokenizerAdapterFallsBackOnErrors(t *testing.T) {
	adapter := NewSharedTokenizerAdapter(sharedTokenizerStub{countErr: true, encodeErr: true}, nil)

	assert.Equal(t, 2, adapter.CountTokens("12345678"))
	assert.Equal(t, []int{0, 1}, adapter.Encode("12345678"))
}

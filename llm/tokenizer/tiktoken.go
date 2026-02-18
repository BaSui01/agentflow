package tokenizer

import (
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// TiktokenTokenizer adapts tiktoken for OpenAI-family models.
type TiktokenTokenizer struct {
	model     string
	encoding  string
	maxTokens int
	enc       *tiktoken.Tiktoken
	once      sync.Once
	initErr   error
}

// modelEncodings maps model names to their tiktoken encoding and context size.
var modelEncodings = map[string]struct {
	encoding  string
	maxTokens int
}{
	"gpt-4o":                    {encoding: "o200k_base", maxTokens: 128000},
	"gpt-4o-mini":               {encoding: "o200k_base", maxTokens: 128000},
	"gpt-4-turbo":               {encoding: "cl100k_base", maxTokens: 128000},
	"gpt-4":                     {encoding: "cl100k_base", maxTokens: 8192},
	"gpt-3.5-turbo":             {encoding: "cl100k_base", maxTokens: 16385},
	"text-embedding-3-large":    {encoding: "cl100k_base", maxTokens: 8191},
	"text-embedding-3-small":    {encoding: "cl100k_base", maxTokens: 8191},
}

// NewTiktokenTokenizer creates a tiktoken-based tokenizer for the given model.
func NewTiktokenTokenizer(model string) (*TiktokenTokenizer, error) {
	info, ok := modelEncodings[model]
	if !ok {
		// Try prefix matching.
		for prefix, i := range modelEncodings {
			if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
				info = i
				ok = true
				break
			}
		}
	}

	if !ok {
		// Default to cl100k_base.
		info = struct {
			encoding  string
			maxTokens int
		}{encoding: "cl100k_base", maxTokens: 8192}
	}

	return &TiktokenTokenizer{
		model:     model,
		encoding:  info.encoding,
		maxTokens: info.maxTokens,
	}, nil
}

// init lazily initializes the tiktoken encoding (may download data on first use).
func (t *TiktokenTokenizer) init() error {
	t.once.Do(func() {
		enc, err := tiktoken.GetEncoding(t.encoding)
		if err != nil {
			t.initErr = fmt.Errorf("init tiktoken encoding %s: %w", t.encoding, err)
			return
		}
		t.enc = enc
	})
	return t.initErr
}

func (t *TiktokenTokenizer) CountTokens(text string) (int, error) {
	if err := t.init(); err != nil {
		return 0, err
	}
	tokens := t.enc.Encode(text, nil, nil)
	return len(tokens), nil
}

func (t *TiktokenTokenizer) CountMessages(messages []Message) (int, error) {
	if err := t.init(); err != nil {
		return 0, err
	}

	total := 0
	for _, msg := range messages {
		// Per-message overhead: <|start|>role\n content <|end|>\n
		total += 4
		tokens := t.enc.Encode(msg.Content, nil, nil)
		total += len(tokens)
		roleTokens := t.enc.Encode(msg.Role, nil, nil)
		total += len(roleTokens)
	}
	total += 3 // conversation-end overhead
	return total, nil
}

func (t *TiktokenTokenizer) Encode(text string) ([]int, error) {
	if err := t.init(); err != nil {
		return nil, err
	}
	return t.enc.Encode(text, nil, nil), nil
}

func (t *TiktokenTokenizer) Decode(tokens []int) (string, error) {
	if err := t.init(); err != nil {
		return "", err
	}
	return t.enc.Decode(tokens), nil
}

func (t *TiktokenTokenizer) MaxTokens() int {
	return t.maxTokens
}

func (t *TiktokenTokenizer) Name() string {
	return fmt.Sprintf("tiktoken[%s]", t.encoding)
}

// RegisterOpenAITokenizers registers tokenizers for all known OpenAI models.
func RegisterOpenAITokenizers() {
	for model := range modelEncodings {
		t, err := NewTiktokenTokenizer(model)
		if err != nil {
			continue
		}
		RegisterTokenizer(model, t)
	}
}

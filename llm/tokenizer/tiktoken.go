package tokenizer

import (
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// TiktokenTokenizer为OpenAI-家庭模型改造tiktoken.
type TiktokenTokenizer struct {
	model     string
	encoding  string
	maxTokens int
	enc       *tiktoken.Tiktoken
	once      sync.Once
	initErr   error
}

// 模型编码将模型名称映射到其tiktoken编码和上下文大小。
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

// NewTiktokenTokenizer为给定型号创建了以tiktoken为主的代号.
func NewTiktokenTokenizer(model string) (*TiktokenTokenizer, error) {
	info, ok := modelEncodings[model]
	if !ok {
		// 尝试前缀匹配 。
		for prefix, i := range modelEncodings {
			if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
				info = i
				ok = true
				break
			}
		}
	}

	if !ok {
		// 默认为 Cl100k  base 。
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

// init lazily 初始化 tiktoken 编码(可以在第一次使用时下载数据).
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
		// 每条消息的开销: <|start|>role\n content<|end|>\n
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

// 注册 OpenAI 用户登记所有已知的 OpenAI 模型的标识器。
func RegisterOpenAITokenizers() {
	for model := range modelEncodings {
		t, err := NewTiktokenTokenizer(model)
		if err != nil {
			continue
		}
		RegisterTokenizer(model, t)
	}
}

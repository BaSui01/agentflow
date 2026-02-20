package tokenizer

import (
	"fmt"
	"sync"
)

// Tokenizer是统一的代号计数界面.
type Tokenizer interface {
	// 伯爵托肯斯返回给定文本中的符号数.
	CountTokens(text string) (int, error)

	// CounterMessages 返回信件列表的总符号数,
	// 包括每条消息的间接费用(作用标记、分离器等)。
	CountMessages(messages []Message) (int, error)

	// Encode 将文本转换为符号ID列表.
	Encode(text string) ([]int, error)

	// 解码后将符号ID转换回文本.
	Decode(tokens []int) (string, error)

	// MaxTokens返回模型的最大上下文长度.
	MaxTokens() int

	// 名称返回一个人类可读的标致器名.
	Name() string
}

// 信件是一个轻量级信件, 由指示器包使用
// 以避免与 llm 包的循环依赖。
type Message struct {
	Role    string
	Content string
}

// 全球标致器注册.
var (
	modelTokenizers   = make(map[string]Tokenizer)
	modelTokenizersMu sync.RWMutex
)

// RegisterTokenizer 为给定的型号名称注册了一个标注符.
func RegisterTokenizer(model string, t Tokenizer) {
	modelTokenizersMu.Lock()
	defer modelTokenizersMu.Unlock()
	modelTokenizers[model] = t
}

// GetTokenizer 返回为给定型号注册的标定器 。
// 它也尝试了前缀匹配(如"gpt-4o"匹配"gpt-4o-mini").
func GetTokenizer(model string) (Tokenizer, error) {
	modelTokenizersMu.RLock()
	defer modelTokenizersMu.RUnlock()

	if t, ok := modelTokenizers[model]; ok {
		return t, nil
	}

	// 尝试前缀匹配 。
	for prefix, t := range modelTokenizers {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return t, nil
		}
	}

	return nil, fmt.Errorf("no tokenizer registered for model: %s", model)
}

// GetTokenizer OrEstimator 返回该模型的注册代号器,
// 如果没有登记,则回到一般估计器。
func GetTokenizerOrEstimator(model string) Tokenizer {
	t, err := GetTokenizer(model)
	if err != nil {
		return NewEstimatorTokenizer(model, 0)
	}
	return t
}

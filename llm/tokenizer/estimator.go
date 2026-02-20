package tokenizer

import (
	"fmt"
	"unicode/utf8"
)

// 估计器Tokenizer是一个基于字符计数的符号估计器.
// 它区分 CJK 和 ASCII 字符, 使其更准确
// 与天真的Len/4方法相比
type EstimatorTokenizer struct {
	model     string
	maxTokens int

	// charsPerToken是默认比率(仅用作倒计时).
	charsPerToken float64
}

// 新估计器Tokenizer创建了通用估计器.
func NewEstimatorTokenizer(model string, maxTokens int) *EstimatorTokenizer {
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	return &EstimatorTokenizer{
		model:         model,
		maxTokens:     maxTokens,
		charsPerToken: 2.5,
	}
}

// 使用 CharsPerToken 来覆盖默认的字符/ 每个字的比值 。
func (e *EstimatorTokenizer) WithCharsPerToken(ratio float64) *EstimatorTokenizer {
	e.charsPerToken = ratio
	return e
}

func (e *EstimatorTokenizer) CountTokens(text string) (int, error) {
	if text == "" {
		return 0, nil
	}

	totalChars := utf8.RuneCountInString(text)
	cjkCount := 0
	for _, r := range text {
		if isCJK(r) {
			cjkCount++
		}
	}

	// CJK字符~1.5个字符/托克,ASCII~4个字符/托克.
	cjkTokens := float64(cjkCount) / 1.5
	asciiTokens := float64(totalChars-cjkCount) / 4.0
	estimated := int(cjkTokens + asciiTokens)

	if estimated == 0 {
		estimated = 1
	}
	return estimated, nil
}

func (e *EstimatorTokenizer) CountMessages(messages []Message) (int, error) {
	total := 0
	for _, msg := range messages {
		// 每条消息都有~4个标语(作用标记,分出符).
		tokens, err := e.CountTokens(msg.Content)
		if err != nil {
			return 0, err
		}
		total += tokens + 4
	}
	// 对话端上方。
	total += 3
	return total, nil
}

func (e *EstimatorTokenizer) Encode(text string) ([]int, error) {
	// 估计器不能真正编码; 返回伪代号 ID 。
	count, err := e.CountTokens(text)
	if err != nil {
		return nil, err
	}
	tokens := make([]int, count)
	for i := range tokens {
		tokens[i] = i
	}
	return tokens, nil
}

func (e *EstimatorTokenizer) Decode(_ []int) (string, error) {
	return "", fmt.Errorf("estimator tokenizer does not support decode")
}

func (e *EstimatorTokenizer) MaxTokens() int {
	return e.maxTokens
}

func (e *EstimatorTokenizer) Name() string {
	return "estimator"
}

// 如果 une 是 CJK 字符, 则返回 true 。
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // Halfwidth and Fullwidth Forms
}

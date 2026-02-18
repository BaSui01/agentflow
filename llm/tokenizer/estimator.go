package tokenizer

import (
	"fmt"
	"unicode/utf8"
)

// EstimatorTokenizer is a character-count-based token estimator.
// It distinguishes CJK and ASCII characters for better accuracy
// compared to a naive len/4 approach.
type EstimatorTokenizer struct {
	model     string
	maxTokens int

	// charsPerToken is the default ratio (used only as a fallback).
	charsPerToken float64
}

// NewEstimatorTokenizer creates a generic estimator.
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

// WithCharsPerToken overrides the default chars-per-token ratio.
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

	// CJK characters ~1.5 chars/token, ASCII ~4 chars/token.
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
		// Each message has ~4 tokens of overhead (role markers, separators).
		tokens, err := e.CountTokens(msg.Content)
		if err != nil {
			return 0, err
		}
		total += tokens + 4
	}
	// Conversation-end overhead.
	total += 3
	return total, nil
}

func (e *EstimatorTokenizer) Encode(text string) ([]int, error) {
	// The estimator cannot truly encode; return pseudo token IDs.
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

// isCJK returns true if the rune is a CJK character.
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x20000 && r <= 0x2A6DF) || // CJK Extension B
		(r >= 0xF900 && r <= 0xFAFF) || // CJK Compatibility Ideographs
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // Halfwidth and Fullwidth Forms
}

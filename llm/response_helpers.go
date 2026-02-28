package llm

import "fmt"

// FirstChoice safely returns the first choice from a ChatResponse.
// Returns an error if the response is nil or has no choices.
func FirstChoice(resp *ChatResponse) (ChatChoice, error) {
	if resp == nil {
		return ChatChoice{}, fmt.Errorf("nil ChatResponse")
	}
	if len(resp.Choices) == 0 {
		return ChatChoice{}, fmt.Errorf("empty choices in ChatResponse (model returned no choices)")
	}
	return resp.Choices[0], nil
}

// Deprecated: MustFirstChoice panics on empty choices. Use FirstChoice() instead.
//
// This function is init-only: use it in main() or init() where a missing choice
// is truly unrecoverable. For runtime code, use FirstChoice() which returns an error.
func MustFirstChoice(resp *ChatResponse) ChatChoice {
	choice, err := FirstChoice(resp)
	if err != nil {
		panic(err)
	}
	return choice
}

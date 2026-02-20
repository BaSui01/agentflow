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

// MustFirstChoice returns the first choice or panics.
// Use only in contexts where empty choices is truly unexpected.
func MustFirstChoice(resp *ChatResponse) ChatChoice {
	choice, err := FirstChoice(resp)
	if err != nil {
		panic(err)
	}
	return choice
}

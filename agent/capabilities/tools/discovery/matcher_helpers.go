package discovery

import (
	"math"
	"strings"
)

// CapabilityMatches reports whether a capability name satisfies a required
// capability expression using exact, prefix, or contains matching.
func CapabilityMatches(capName, required string) bool {
	capName = strings.ToLower(capName)
	required = strings.ToLower(required)
	if strings.EqualFold(capName, required) {
		return true
	}
	if strings.HasPrefix(capName, required) {
		return true
	}
	return strings.Contains(capName, required)
}

// TokenizeForSemanticMatch splits text into normalized semantic tokens.
func TokenizeForSemanticMatch(text string) []string {
	text = strings.ToLower(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "as": true, "at": true, "by": true, "from": true,
		"this": true, "that": true, "it": true, "its": true,
	}

	filtered := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) > 2 && !stopWords[w] {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

// IsExcludedAgent reports whether agentID is in an exclusion list.
func IsExcludedAgent(agentID string, excluded []string) bool {
	for _, ex := range excluded {
		if ex == agentID {
			return true
		}
	}
	return false
}

// SemanticScore calculates keyword-based similarity between a task description
// and the agent plus capability descriptions.
func SemanticScore(agentDescription string, capabilityDescriptions []string, taskDescription string) (semanticScore float64, coverage float64) {
	taskWords := TokenizeForSemanticMatch(taskDescription)
	if len(taskWords) == 0 {
		return 0, 0
	}

	matchCount := 0
	agentWords := TokenizeForSemanticMatch(agentDescription)
	for _, taskWord := range taskWords {
		for _, agentWord := range agentWords {
			if strings.EqualFold(taskWord, agentWord) {
				matchCount++
				break
			}
		}
	}

	for _, description := range capabilityDescriptions {
		capabilityWords := TokenizeForSemanticMatch(description)
		for _, taskWord := range taskWords {
			for _, capabilityWord := range capabilityWords {
				if strings.EqualFold(taskWord, capabilityWord) {
					matchCount++
					break
				}
			}
		}
	}

	if matchCount == 0 {
		return 0, 0
	}

	score := math.Min(1.0, float64(matchCount)/float64(len(taskWords)))
	confidence := math.Min(1.0, float64(matchCount)/5.0)
	return score, confidence
}

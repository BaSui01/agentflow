package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSemanticScoreMatchesAgentAndCapabilityDescriptions(t *testing.T) {
	score, confidence := SemanticScore(
		"agent can review code and summarize pull requests",
		[]string{"go code review", "database migration"},
		"review Go code",
	)

	assert.InDelta(t, 1.0, score, 0.0001)
	assert.InDelta(t, 0.8, confidence, 0.0001)
}

func TestSemanticScoreReturnsZeroForOnlyStopWords(t *testing.T) {
	score, confidence := SemanticScore("anything", []string{"anything"}, "the and for")

	assert.Zero(t, score)
	assert.Zero(t, confidence)
}

func TestSemanticScoreCapsScoreAndConfidence(t *testing.T) {
	score, confidence := SemanticScore(
		"alpha beta gamma delta epsilon zeta",
		[]string{"alpha beta gamma delta epsilon zeta"},
		"alpha beta gamma delta epsilon zeta",
	)

	assert.Equal(t, 1.0, score)
	assert.Equal(t, 1.0, confidence)
}

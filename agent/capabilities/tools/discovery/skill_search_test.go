package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTokenizeSkillQueryDeduplicatesAndKeepsUsefulSeparators(t *testing.T) {
	tokens := TokenizeSkillQuery("Go code-review, go_data!")

	assert.Equal(t, []string{"go", "code-review", "go_data"}, tokens)
}

func TestScoreSkillMetadataMatchUsesNameDescriptionCategoryTagsAndTokens(t *testing.T) {
	profile := SkillSearchProfile{
		Name:        "Code Review",
		Description: "Review Go HTTP handlers",
		Category:    "coding",
		Tags:        []string{"golang", "quality"},
	}

	score := ScoreSkillMetadataMatch(profile, "go quality", TokenizeSkillQuery("go quality"))

	assert.InDelta(t, 0.4, score, 0.0001)
}

func TestSortSkillSearchResultsOrdersByScoreThenName(t *testing.T) {
	results := []SkillSearchResult{
		{ID: "b", Name: "Beta", Score: 0.5},
		{ID: "c", Name: "Alpha", Score: 0.9},
		{ID: "a", Name: "Alpha", Score: 0.5},
	}

	SortSkillSearchResults(results)

	assert.Equal(t, []SkillSearchResult{{ID: "c", Name: "Alpha", Score: 0.9}, {ID: "a", Name: "Alpha", Score: 0.5}, {ID: "b", Name: "Beta", Score: 0.5}}, results)
}

func TestScoreSkillProfileMatchMatchesLegacySkillFormula(t *testing.T) {
	profile := SkillSearchProfile{
		Name:        "Code Review",
		Description: "Review code quality and safety",
		Category:    "development",
		Tags:        []string{"go", "quality"},
	}

	score := ScoreSkillProfileMatch(profile, "code review go quality")

	// name=0.3, description words code/review/quality=0.3, tags go+quality=0.2
	assert.InDelta(t, 0.8, score, 0.0001)
}

func TestScoreSkillProfileMatchHandlesEmptyTask(t *testing.T) {
	assert.Zero(t, ScoreSkillProfileMatch(SkillSearchProfile{Name: "Code Review"}, ""))
}

package discovery

import (
	"sort"
	"strings"
	"unicode"
)

// SkillSearchProfile is the discovery-layer view of searchable skill metadata.
type SkillSearchProfile struct {
	Name        string
	Description string
	Category    string
	Tags        []string
}

// SkillSearchResult is the sortable view of a skill search hit.
type SkillSearchResult struct {
	ID    string
	Name  string
	Score float64
}

// TokenizeSkillQuery splits a search query into normalized unique tokens.
func TokenizeSkillQuery(query string) []string {
	if query == "" {
		return nil
	}
	tokens := strings.FieldsFunc(query, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
	})
	if len(tokens) == 0 {
		return nil
	}

	unique := make(map[string]struct{}, len(tokens))
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.ToLower(strings.TrimSpace(token))
		if token == "" {
			continue
		}
		if _, exists := unique[token]; exists {
			continue
		}
		unique[token] = struct{}{}
		result = append(result, token)
	}
	return result
}

// ScoreSkillMetadataMatch scores how well searchable skill metadata matches a query.
func ScoreSkillMetadataMatch(profile SkillSearchProfile, query string, tokens []string) float64 {
	if query == "" {
		return 1
	}

	name := strings.ToLower(profile.Name)
	description := strings.ToLower(profile.Description)
	category := strings.ToLower(profile.Category)

	score := 0.0
	if strings.Contains(name, query) {
		score += 0.45
	}
	if strings.Contains(description, query) {
		score += 0.25
	}
	if strings.Contains(category, query) {
		score += 0.15
	}

	if skillTagsContain(profile.Tags, query) {
		score += 0.15
	}

	if len(tokens) > 0 {
		matched := 0
		for _, token := range tokens {
			if strings.Contains(name, token) || strings.Contains(description, token) || strings.Contains(category, token) || skillTagsContain(profile.Tags, token) {
				matched++
			}
		}
		score += 0.4 * float64(matched) / float64(len(tokens))
	}

	if score > 1 {
		return 1
	}
	return score
}

// ScoreSkillProfileMatch scores a loaded skill profile against a natural-language task.
func ScoreSkillProfileMatch(profile SkillSearchProfile, task string) float64 {
	task = strings.ToLower(task)
	score := 0.0

	if strings.Contains(task, strings.ToLower(profile.Name)) {
		score += 0.3
	}

	descWords := strings.Fields(strings.ToLower(profile.Description))
	taskWords := strings.Fields(task)

	matchCount := 0
	for _, taskWord := range taskWords {
		for _, descWord := range descWords {
			if taskWord == descWord || strings.Contains(descWord, taskWord) || strings.Contains(taskWord, descWord) {
				matchCount++
				break
			}
		}
	}

	if len(taskWords) > 0 {
		score += 0.4 * float64(matchCount) / float64(len(taskWords))
	}

	for _, tag := range profile.Tags {
		if strings.Contains(task, strings.ToLower(tag)) {
			score += 0.1
		}
	}

	if profile.Category != "" && strings.Contains(task, strings.ToLower(profile.Category)) {
		score += 0.2
	}

	return score
}

// SortSkillSearchResults orders search hits by score descending, then name ascending.
func SortSkillSearchResults(results []SkillSearchResult) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		if results[i].Name != results[j].Name {
			return results[i].Name < results[j].Name
		}
		return results[i].ID < results[j].ID
	})
}

func skillTagsContain(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

package sources

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewArxivSource_DefaultConfig(t *testing.T) {
	t.Parallel()

	config := DefaultArxivConfig()
	logger, _ := zap.NewDevelopment()
	src := NewArxivSource(config, logger)

	assert.NotNil(t, src)
	assert.Equal(t, "arxiv", src.Name())
	assert.Equal(t, config.BaseURL, src.config.BaseURL)
	assert.Equal(t, config.MaxResults, src.config.MaxResults)
	assert.Equal(t, config.Timeout, src.config.Timeout)
}

func TestNewArxivSource_NilLogger(t *testing.T) {
	t.Parallel()

	config := DefaultArxivConfig()
	assert.NotPanics(t, func() {
		src := NewArxivSource(config, nil)
		assert.NotNil(t, src)
		assert.NotNil(t, src.logger)
	})
}

func TestDefaultArxivConfig_SensibleValues(t *testing.T) {
	t.Parallel()

	config := DefaultArxivConfig()

	assert.Equal(t, "http://export.arxiv.org/api/query", config.BaseURL)
	assert.Equal(t, 20, config.MaxResults)
	assert.Equal(t, "relevance", config.SortBy)
	assert.Equal(t, "descending", config.SortOrder)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.RetryCount)
	assert.Equal(t, 2*time.Second, config.RetryDelay)
	assert.Empty(t, config.Categories)
}

func TestArxivSource_BuildQuery_NoCategories(t *testing.T) {
	t.Parallel()

	config := DefaultArxivConfig()
	src := NewArxivSource(config, nil)

	result := src.buildQuery("transformer attention")
	assert.Equal(t, "all:transformer attention", result)
}

func TestArxivSource_BuildQuery_WithSingleCategory(t *testing.T) {
	t.Parallel()

	config := DefaultArxivConfig()
	config.Categories = []string{"cs.AI"}
	src := NewArxivSource(config, nil)

	result := src.buildQuery("neural networks")
	assert.Contains(t, result, "all:neural networks")
	assert.Contains(t, result, "+AND+")
	assert.Contains(t, result, "cat:cs.AI")
}

func TestArxivSource_BuildQuery_WithMultipleCategories(t *testing.T) {
	t.Parallel()

	config := DefaultArxivConfig()
	config.Categories = []string{"cs.AI", "cs.CL", "cs.LG"}
	src := NewArxivSource(config, nil)

	result := src.buildQuery("deep learning")
	assert.Contains(t, result, "all:deep learning")
	assert.Contains(t, result, "+AND+")
	assert.Contains(t, result, "cat:cs.AI")
	assert.Contains(t, result, "+OR+")
	assert.Contains(t, result, "cat:cs.CL")
	assert.Contains(t, result, "cat:cs.LG")
	// Verify the category part is wrapped in parentheses
	assert.Contains(t, result, "(cat:cs.AI+OR+cat:cs.CL+OR+cat:cs.LG)")
}

func TestArxivSource_ToJSON(t *testing.T) {
	t.Parallel()

	src := NewArxivSource(DefaultArxivConfig(), nil)

	published := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	updated := time.Date(2024, 2, 1, 10, 0, 0, 0, time.UTC)

	papers := []ArxivPaper{
		{
			ID:          "2401.12345",
			Title:       "Test Paper on Transformers",
			Summary:     "A summary of the paper.",
			Authors:     []string{"Alice", "Bob"},
			Categories:  []string{"cs.AI", "cs.CL"},
			Published:   published,
			Updated:     updated,
			PDFURL:      "https://arxiv.org/pdf/2401.12345",
			AbstractURL: "https://arxiv.org/abs/2401.12345",
			DOI:         "10.1234/test",
		},
	}

	jsonStr, err := src.ToJSON(papers)
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonStr)

	// Verify it's valid JSON by unmarshalling back
	var decoded []ArxivPaper
	err = json.Unmarshal([]byte(jsonStr), &decoded)
	assert.NoError(t, err)
	assert.Len(t, decoded, 1)
	assert.Equal(t, "2401.12345", decoded[0].ID)
	assert.Equal(t, "Test Paper on Transformers", decoded[0].Title)
	assert.Equal(t, []string{"Alice", "Bob"}, decoded[0].Authors)
	assert.Equal(t, "10.1234/test", decoded[0].DOI)
}

func TestArxivSource_ToJSON_EmptySlice(t *testing.T) {
	t.Parallel()

	src := NewArxivSource(DefaultArxivConfig(), nil)

	jsonStr, err := src.ToJSON([]ArxivPaper{})
	assert.NoError(t, err)
	assert.Equal(t, "[]", jsonStr)
}

func TestNewArxivSource_CustomConfig(t *testing.T) {
	t.Parallel()

	config := ArxivConfig{
		BaseURL:    "http://custom.arxiv.org/api",
		MaxResults: 50,
		SortBy:     "submittedDate",
		SortOrder:  "ascending",
		Timeout:    10 * time.Second,
		RetryCount: 5,
		RetryDelay: 1 * time.Second,
		Categories: []string{"cs.AI"},
	}
	src := NewArxivSource(config, nil)

	assert.Equal(t, "http://custom.arxiv.org/api", src.config.BaseURL)
	assert.Equal(t, 50, src.config.MaxResults)
	assert.Equal(t, 5, src.config.RetryCount)
	assert.Equal(t, []string{"cs.AI"}, src.config.Categories)
}

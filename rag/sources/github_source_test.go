package sources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestNewGitHubSource_DefaultConfig(t *testing.T) {
	t.Parallel()

	config := DefaultGitHubConfig()
	logger, _ := zap.NewDevelopment()
	src := NewGitHubSource(config, logger)

	assert.NotNil(t, src)
	assert.Equal(t, "github", src.Name())
	assert.Equal(t, config.BaseURL, src.config.BaseURL)
	assert.Equal(t, config.MaxResults, src.config.MaxResults)
	assert.Equal(t, config.Timeout, src.config.Timeout)
}

func TestNewGitHubSource_NilLogger(t *testing.T) {
	t.Parallel()

	config := DefaultGitHubConfig()
	assert.NotPanics(t, func() {
		src := NewGitHubSource(config, nil)
		assert.NotNil(t, src)
		assert.NotNil(t, src.logger)
	})
}

func TestDefaultGitHubConfig_SensibleValues(t *testing.T) {
	t.Parallel()

	config := DefaultGitHubConfig()

	assert.Equal(t, "https://api.github.com", config.BaseURL)
	assert.Equal(t, 20, config.MaxResults)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 3, config.RetryCount)
	assert.Equal(t, 2*time.Second, config.RetryDelay)
}

func TestFilterByStars(t *testing.T) {
	t.Parallel()

	repos := []GitHubRepo{
		{FullName: "user/repo-a", Stars: 100},
		{FullName: "user/repo-b", Stars: 500},
		{FullName: "user/repo-c", Stars: 50},
		{FullName: "user/repo-d", Stars: 1000},
		{FullName: "user/repo-e", Stars: 0},
	}

	t.Run("filter above 100", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByStars(repos, 100)
		assert.Len(t, filtered, 3)
		for _, r := range filtered {
			assert.GreaterOrEqual(t, r.Stars, 100)
		}
	})

	t.Run("filter above 500", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByStars(repos, 500)
		assert.Len(t, filtered, 2)
		assert.Equal(t, "user/repo-b", filtered[0].FullName)
		assert.Equal(t, "user/repo-d", filtered[1].FullName)
	})

	t.Run("filter above 0 returns all", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByStars(repos, 0)
		assert.Len(t, filtered, 5)
	})

	t.Run("filter above max returns none", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByStars(repos, 5000)
		assert.Empty(t, filtered)
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByStars(nil, 10)
		assert.Empty(t, filtered)
	})
}

func TestFilterByLanguage(t *testing.T) {
	t.Parallel()

	repos := []GitHubRepo{
		{FullName: "user/go-project", Language: "Go"},
		{FullName: "user/python-project", Language: "Python"},
		{FullName: "user/another-go", Language: "Go"},
		{FullName: "user/rust-project", Language: "Rust"},
		{FullName: "user/no-lang", Language: ""},
	}

	t.Run("filter Go", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByLanguage(repos, "Go")
		assert.Len(t, filtered, 2)
		for _, r := range filtered {
			assert.Equal(t, "Go", r.Language)
		}
	})

	t.Run("case insensitive match", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByLanguage(repos, "go")
		assert.Len(t, filtered, 2)

		filtered2 := FilterByLanguage(repos, "GO")
		assert.Len(t, filtered2, 2)

		filtered3 := FilterByLanguage(repos, "python")
		assert.Len(t, filtered3, 1)
		assert.Equal(t, "user/python-project", filtered3[0].FullName)
	})

	t.Run("no match", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByLanguage(repos, "Java")
		assert.Empty(t, filtered)
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByLanguage(nil, "Go")
		assert.Empty(t, filtered)
	})

	t.Run("empty language filter matches empty language repos", func(t *testing.T) {
		t.Parallel()
		filtered := FilterByLanguage(repos, "")
		assert.Len(t, filtered, 1)
		assert.Equal(t, "user/no-lang", filtered[0].FullName)
	})
}

func TestNewGitHubSource_CustomConfig(t *testing.T) {
	t.Parallel()

	config := GitHubConfig{
		BaseURL:    "https://custom.github.com/api",
		Token:      "test-token",
		MaxResults: 50,
		Timeout:    10 * time.Second,
		RetryCount: 5,
		RetryDelay: 1 * time.Second,
	}
	src := NewGitHubSource(config, nil)

	assert.NotNil(t, src)
	assert.Equal(t, "https://custom.github.com/api", src.config.BaseURL)
	assert.Equal(t, "test-token", src.config.Token)
	assert.Equal(t, 50, src.config.MaxResults)
}

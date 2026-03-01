package sources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// GitHubSource — SearchRepos with httptest
// ============================================================

func TestGitHubSource_SearchRepos_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/vnd.github.v3+json", r.Header.Get("Accept"))
		resp := GitHubSearchResponse{
			TotalCount: 2,
			Items: []GitHubRepo{
				{FullName: "user/repo-a", Stars: 100, Language: "Go"},
				{FullName: "user/repo-b", Stars: 50, Language: "Python"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{
		BaseURL:    srv.URL,
		MaxResults: 10,
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}
	src := NewGitHubSource(config, zap.NewNop())

	repos, err := src.SearchRepos(context.Background(), "test", 0)
	require.NoError(t, err)
	require.Len(t, repos, 2)
	assert.Equal(t, "user/repo-a", repos[0].FullName)
	assert.Equal(t, 100, repos[0].Stars)
}

func TestGitHubSource_SearchRepos_WithToken(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token-123", r.Header.Get("Authorization"))
		resp := GitHubSearchResponse{TotalCount: 0, Items: []GitHubRepo{}}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{
		BaseURL:    srv.URL,
		Token:      "test-token-123",
		MaxResults: 5,
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}
	src := NewGitHubSource(config, zap.NewNop())

	repos, err := src.SearchRepos(context.Background(), "test", 5)
	require.NoError(t, err)
	assert.Empty(t, repos)
}

func TestGitHubSource_SearchRepos_ServerError_Retries(t *testing.T) {
	t.Parallel()

	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("server error"))
			return
		}
		resp := GitHubSearchResponse{TotalCount: 1, Items: []GitHubRepo{
			{FullName: "user/repo", Stars: 10},
		}}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{
		BaseURL:    srv.URL,
		MaxResults: 5,
		Timeout:    5 * time.Second,
		RetryCount: 3,
		RetryDelay: 1 * time.Millisecond,
	}
	src := NewGitHubSource(config, zap.NewNop())

	repos, err := src.SearchRepos(context.Background(), "test", 0)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, 3, callCount)
}

func TestGitHubSource_SearchRepos_AllRetriesFail(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{
		BaseURL:    srv.URL,
		MaxResults: 5,
		Timeout:    5 * time.Second,
		RetryCount: 1,
		RetryDelay: 1 * time.Millisecond,
	}
	src := NewGitHubSource(config, zap.NewNop())

	_, err := src.SearchRepos(context.Background(), "test", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after")
}

func TestGitHubSource_SearchRepos_InvalidJSON(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{
		BaseURL:    srv.URL,
		MaxResults: 5,
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}
	src := NewGitHubSource(config, zap.NewNop())

	_, err := src.SearchRepos(context.Background(), "test", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestGitHubSource_SearchRepos_CancelledContext(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{
		BaseURL:    srv.URL,
		MaxResults: 5,
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}
	src := NewGitHubSource(config, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.SearchRepos(ctx, "test", 0)
	require.Error(t, err)
}

// ============================================================
// GitHubSource — SearchCode with httptest
// ============================================================

func TestGitHubSource_SearchCode_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		assert.Contains(t, q, "language:go")
		resp := struct {
			TotalCount int                `json:"total_count"`
			Items      []GitHubCodeResult `json:"items"`
		}{
			TotalCount: 1,
			Items: []GitHubCodeResult{
				{Name: "main.go", Path: "cmd/main.go", Score: 1.0},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{
		BaseURL:    srv.URL,
		MaxResults: 5,
		Timeout:    5 * time.Second,
	}
	src := NewGitHubSource(config, zap.NewNop())

	results, err := src.SearchCode(context.Background(), "func main", "go", 0)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "main.go", results[0].Name)
}

func TestGitHubSource_SearchCode_NoLanguage(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/code", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		assert.NotContains(t, q, "language:")
		resp := struct {
			TotalCount int                `json:"total_count"`
			Items      []GitHubCodeResult `json:"items"`
		}{TotalCount: 0, Items: []GitHubCodeResult{}}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{BaseURL: srv.URL, MaxResults: 5, Timeout: 5 * time.Second}
	src := NewGitHubSource(config, zap.NewNop())

	results, err := src.SearchCode(context.Background(), "test", "", 5)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// ============================================================
// GitHubSource — GetReadme with httptest
// ============================================================

func TestGitHubSource_GetReadme_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/readme", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/vnd.github.raw+json", r.Header.Get("Accept"))
		_, _ = w.Write([]byte("# My Project\n\nThis is the readme."))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{BaseURL: srv.URL, Timeout: 5 * time.Second}
	src := NewGitHubSource(config, zap.NewNop())

	readme, err := src.GetReadme(context.Background(), "owner", "repo")
	require.NoError(t, err)
	assert.Contains(t, readme, "My Project")
}

func TestGitHubSource_GetReadme_NotFound(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/readme", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{BaseURL: srv.URL, Timeout: 5 * time.Second}
	src := NewGitHubSource(config, zap.NewNop())

	_, err := src.GetReadme(context.Background(), "owner", "repo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

// ============================================================
// GitHubSource — SearchRepos with license extraction
// ============================================================

func TestGitHubSource_SearchRepos_ExtractsLicense(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/search/repositories", func(w http.ResponseWriter, r *http.Request) {
		// Return raw JSON with nested license object
		_, _ = w.Write([]byte(`{
			"total_count": 1,
			"items": [{
				"full_name": "user/repo",
				"stargazers_count": 100,
				"license": {"name": "MIT License", "spdx_id": "MIT"}
			}]
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	config := GitHubConfig{BaseURL: srv.URL, MaxResults: 5, Timeout: 5 * time.Second, RetryCount: 0}
	src := NewGitHubSource(config, zap.NewNop())

	repos, err := src.SearchRepos(context.Background(), "test", 0)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "MIT License", repos[0].License)
}


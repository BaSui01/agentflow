package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================
// ArxivSource — Search with httptest
// ============================================================

func TestArxivSource_Search_Success(t *testing.T) {
	t.Parallel()

	atomXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2401.12345v1</id>
    <title>Test Paper on Transformers</title>
    <summary>A summary about transformers.</summary>
    <published>2024-01-15T12:00:00Z</published>
    <updated>2024-02-01T10:00:00Z</updated>
    <author><name>Alice</name></author>
    <author><name>Bob</name></author>
    <link href="http://arxiv.org/abs/2401.12345v1" rel="alternate" type="text/html"/>
    <link href="http://arxiv.org/pdf/2401.12345v1" type="application/pdf" title="pdf"/>
    <category term="cs.AI"/>
    <category term="cs.CL"/>
  </entry>
</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(atomXML))
	}))
	defer srv.Close()

	config := ArxivConfig{
		BaseURL:    srv.URL,
		MaxResults: 10,
		SortBy:     "relevance",
		SortOrder:  "descending",
		Timeout:    5 * time.Second,
		RetryCount: 0,
	}
	src := NewArxivSource(config, zap.NewNop())

	papers, err := src.Search(context.Background(), "transformers", 0)
	require.NoError(t, err)
	require.Len(t, papers, 1)

	p := papers[0]
	assert.Contains(t, p.ID, "2401.12345")
	assert.Equal(t, "Test Paper on Transformers", p.Title)
	assert.Contains(t, p.Summary, "transformers")
	assert.Equal(t, []string{"Alice", "Bob"}, p.Authors)
	assert.Equal(t, []string{"cs.AI", "cs.CL"}, p.Categories)
	assert.Contains(t, p.PDFURL, "pdf")
	assert.Contains(t, p.AbstractURL, "abs")
	assert.False(t, p.Published.IsZero())
	assert.False(t, p.Updated.IsZero())
}

func TestArxivSource_Search_EmptyResults(t *testing.T) {
	t.Parallel()

	atomXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(atomXML))
	}))
	defer srv.Close()

	config := ArxivConfig{BaseURL: srv.URL, MaxResults: 10, SortBy: "relevance", SortOrder: "descending", Timeout: 5 * time.Second, RetryCount: 0}
	src := NewArxivSource(config, zap.NewNop())

	papers, err := src.Search(context.Background(), "nonexistent", 0)
	require.NoError(t, err)
	assert.Empty(t, papers)
}

func TestArxivSource_Search_ServerError_Retries(t *testing.T) {
	t.Parallel()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>1</id><title>Paper</title><summary>Sum</summary></entry></feed>`))
	}))
	defer srv.Close()

	config := ArxivConfig{
		BaseURL:    srv.URL,
		MaxResults: 5,
		SortBy:     "relevance",
		SortOrder:  "descending",
		Timeout:    5 * time.Second,
		RetryCount: 3,
		RetryDelay: 1 * time.Millisecond,
	}
	src := NewArxivSource(config, zap.NewNop())

	papers, err := src.Search(context.Background(), "test", 0)
	require.NoError(t, err)
	require.Len(t, papers, 1)
	assert.Equal(t, 3, callCount)
}

func TestArxivSource_Search_AllRetriesFail(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	config := ArxivConfig{
		BaseURL:    srv.URL,
		MaxResults: 5,
		SortBy:     "relevance",
		SortOrder:  "descending",
		Timeout:    5 * time.Second,
		RetryCount: 1,
		RetryDelay: 1 * time.Millisecond,
	}
	src := NewArxivSource(config, zap.NewNop())

	_, err := src.Search(context.Background(), "test", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed after")
}

func TestArxivSource_Search_InvalidXML(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not xml"))
	}))
	defer srv.Close()

	config := ArxivConfig{BaseURL: srv.URL, MaxResults: 5, SortBy: "relevance", SortOrder: "descending", Timeout: 5 * time.Second, RetryCount: 0}
	src := NewArxivSource(config, zap.NewNop())

	_, err := src.Search(context.Background(), "test", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestArxivSource_Search_CancelledContext(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
	}))
	defer srv.Close()

	config := ArxivConfig{BaseURL: srv.URL, MaxResults: 5, SortBy: "relevance", SortOrder: "descending", Timeout: 5 * time.Second, RetryCount: 0}
	src := NewArxivSource(config, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Search(ctx, "test", 0)
	require.Error(t, err)
}

// ============================================================
// ArxivSource — parseResponse
// ============================================================

func TestArxivSource_ParseResponse_WithDOIAndComment(t *testing.T) {
	t.Parallel()

	atomXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2401.99999v1</id>
    <title>Paper with DOI</title>
    <summary>Summary text</summary>
    <published>2024-03-01T00:00:00Z</published>
    <updated>2024-03-02T00:00:00Z</updated>
    <author><name>Charlie</name></author>
    <doi>10.5555/test-doi</doi>
    <comment>Accepted at NeurIPS 2024</comment>
  </entry>
</feed>`

	src := NewArxivSource(DefaultArxivConfig(), zap.NewNop())
	papers, err := src.parseResponse([]byte(atomXML))
	require.NoError(t, err)
	require.Len(t, papers, 1)
	assert.Equal(t, "10.5555/test-doi", papers[0].DOI)
	assert.Equal(t, "Accepted at NeurIPS 2024", papers[0].Comment)
}

func TestArxivSource_ParseResponse_InvalidDates(t *testing.T) {
	t.Parallel()

	atomXML := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>1</id>
    <title>Paper</title>
    <summary>Sum</summary>
    <published>not-a-date</published>
    <updated>also-not-a-date</updated>
  </entry>
</feed>`

	src := NewArxivSource(DefaultArxivConfig(), zap.NewNop())
	papers, err := src.parseResponse([]byte(atomXML))
	require.NoError(t, err)
	require.Len(t, papers, 1)
	assert.True(t, papers[0].Published.IsZero())
	assert.True(t, papers[0].Updated.IsZero())
}



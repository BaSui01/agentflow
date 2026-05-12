package remote

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiscoveryAgentsURLBuildsFilteredEndpoint(t *testing.T) {
	url := DiscoveryAgentsURL("https://registry.example/root/", DiscoveryQueryFilter{
		Capabilities: []string{"search", "summarize"},
		Tags:         []string{"fast", "reliable"},
	})

	assert.Equal(t, "https://registry.example/root/discovery/agents?capabilities=search,summarize&tags=fast,reliable", url)
}

func TestDiscoveryAgentsURLSkipsEmptyFilter(t *testing.T) {
	assert.Equal(t, "https://registry.example/discovery/agents", DiscoveryAgentsURL("https://registry.example", DiscoveryQueryFilter{}))
}

func TestDiscoveryAnnounceURLTrimsTrailingSlash(t *testing.T) {
	assert.Equal(t, "https://registry.example/discovery/announce", DiscoveryAnnounceURL("https://registry.example/"))
}

func TestSplitAndTrimCSVRemovesEmptyValues(t *testing.T) {
	assert.Equal(t, []string{"search", "summarize"}, SplitAndTrimCSV(" search, , summarize ,,"))
}

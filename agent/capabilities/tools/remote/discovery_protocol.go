package remote

import (
	"bytes"
	"strings"
)

// DiscoveryQueryFilter contains HTTP query fields for discovery agent listing.
type DiscoveryQueryFilter struct {
	Capabilities []string
	Tags         []string
}

// DiscoveryAgentsURL builds the discovery agent listing URL for a remote server.
func DiscoveryAgentsURL(serverURL string, filter DiscoveryQueryFilter) string {
	url := strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/discovery/agents"
	params := make([]string, 0, 2)
	if len(filter.Capabilities) > 0 {
		params = append(params, "capabilities="+JoinStrings(filter.Capabilities, ","))
	}
	if len(filter.Tags) > 0 {
		params = append(params, "tags="+JoinStrings(filter.Tags, ","))
	}
	if len(params) > 0 {
		url += "?" + JoinStrings(params, "&")
	}
	return url
}

// DiscoveryAnnounceURL builds the discovery announcement URL for a remote server.
func DiscoveryAnnounceURL(serverURL string) string {
	return strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/discovery/announce"
}

// SplitAndTrimCSV splits a comma separated query value and drops blank items.
func SplitAndTrimCSV(value string) []string {
	return SplitAndTrim(value, ",")
}

// SplitAndTrim splits a string and trims whitespace from each non-empty part.
func SplitAndTrim(value, sep string) []string {
	parts := make([]string, 0)
	for _, part := range bytes.Split([]byte(value), []byte(sep)) {
		trimmed := bytes.TrimSpace(part)
		if len(trimmed) > 0 {
			parts = append(parts, string(trimmed))
		}
	}
	return parts
}

// JoinStrings joins strings with the provided separator.
func JoinStrings(values []string, sep string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, sep)
}

package tools

import (
	"os"
	"strings"
	"testing"
)

func TestDiscoveryProtocolRemoteHelpersDelegateToRemotePackage(t *testing.T) {
	source, err := os.ReadFile("protocol.go")
	if err != nil {
		t.Fatalf("read protocol.go: %v", err)
	}
	body := string(source)

	for _, want := range []string{
		"toolremote.DiscoveryAgentsURL",
		"toolremote.DiscoveryAnnounceURL",
		"toolremote.SplitAndTrimCSV",
		"toolremote.JoinStrings",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected protocol.go to contain %q", want)
		}
	}
	for _, oldRootLogic := range []string{
		"params := make([]string, 0)",
		"result += sep + strs[i]",
		"bytes.TrimSpace(part)",
	} {
		if strings.Contains(body, oldRootLogic) {
			t.Fatalf("expected remote protocol helper logic to live in remote subpackage, found %q", oldRootLogic)
		}
	}
}

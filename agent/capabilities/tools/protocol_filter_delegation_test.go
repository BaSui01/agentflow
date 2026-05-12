package tools

import (
	"os"
	"strings"
	"testing"
)

func TestDiscoveryProtocolFilterDelegatesToDiscoveryPackage(t *testing.T) {
	source, err := os.ReadFile("protocol.go")
	if err != nil {
		t.Fatalf("read protocol.go: %v", err)
	}
	body := string(source)

	if !strings.Contains(body, "tooldiscovery.MatchesAgentFilter") {
		t.Fatalf("expected protocol.go to delegate filter matching to discovery package")
	}
	for _, oldRootLogic := range []string{
		"for _, status := range filter.Status",
		"for _, reqCap := range filter.Capabilities",
		"for _, reqTag := range filter.Tags",
	} {
		if strings.Contains(body, oldRootLogic) {
			t.Fatalf("expected discovery filter matching logic to live in discovery subpackage, found %q", oldRootLogic)
		}
	}
}

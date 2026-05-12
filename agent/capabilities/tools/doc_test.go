package tools

import (
	"os"
	"strings"
	"testing"
)

func TestToolsPackageDocDescribesCurrentSubpackageDependencyGraph(t *testing.T) {
	source, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatalf("read doc.go: %v", err)
	}
	body := string(source)

	for _, want := range []string{
		"Package tools",
		"# 子包职责与依赖图",
		"registry/",
		"discovery/",
		"execution/",
		"remote/",
		"store/",
		"tools facade",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected tools/doc.go to contain %q", want)
		}
	}
	for _, forbidden := range []string{
		"Package skills",
		"两套并行体系",
		"后续拆分",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("tools/doc.go still contains stale package/split wording %q", forbidden)
		}
	}
}

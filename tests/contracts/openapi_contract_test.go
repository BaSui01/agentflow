package contracts

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestOpenAPIPathsMatchRuntimeRoutes(t *testing.T) {
	repoRoot := resolveRepoRoot(t)

	runtimeRoutes := make(map[string]struct{})
	mergeRouteSet(runtimeRoutes, mustParseHandleFuncRoutes(t, filepath.Join(repoRoot, "cmd", "agentflow", "server.go")))
	mergeRouteSet(runtimeRoutes, mustParseHandleFuncRoutes(t, filepath.Join(repoRoot, "config", "api.go")))

	docRoutes := mustParseOpenAPIPaths(t, filepath.Join(repoRoot, "api", "openapi.yaml"))

	runtimeSorted := sortedRouteKeys(runtimeRoutes)
	docSorted := sortedRouteKeys(docRoutes)

	if !reflect.DeepEqual(runtimeSorted, docSorted) {
		t.Fatalf("openapi paths mismatch runtime routes\nopenapi=%v\nruntime=%v", docSorted, runtimeSorted)
	}
}

func resolveRepoRoot(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
}

func mustParseHandleFuncRoutes(t *testing.T, path string) map[string]struct{} {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open route source %s: %v", path, err)
	}
	defer file.Close()

	routePattern := regexp.MustCompile(`^\s*mux\.HandleFunc\("([^"]+)"`)
	routes := make(map[string]struct{})

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "//") {
			continue
		}
		match := routePattern.FindStringSubmatch(line)
		if len(match) == 2 {
			routes[match[1]] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("scan route source %s: %v", path, err)
	}

	return routes
}

func mustParseOpenAPIPaths(t *testing.T, path string) map[string]struct{} {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read openapi file %s: %v", path, err)
	}

	var doc struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi file %s: %v", path, err)
	}

	routes := make(map[string]struct{}, len(doc.Paths))
	for route := range doc.Paths {
		routes[route] = struct{}{}
	}

	return routes
}

func mergeRouteSet(dst map[string]struct{}, src map[string]struct{}) {
	for route := range src {
		dst[route] = struct{}{}
	}
}

func sortedRouteKeys(routes map[string]struct{}) []string {
	keys := make([]string, 0, len(routes))
	for route := range routes {
		keys = append(keys, route)
	}
	sort.Strings(keys)
	return keys
}

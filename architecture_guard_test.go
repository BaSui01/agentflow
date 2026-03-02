package agentflow_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Keep the root agent package shrinking over time.
// This guard prevents adding more production files to agent/.
func TestAgentRootPackageFileBudget(t *testing.T) {
	const maxAgentRootFiles = 36

	entries, err := os.ReadDir("agent")
	if err != nil {
		t.Fatalf("read agent dir: %v", err)
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		count++
	}

	if count > maxAgentRootFiles {
		t.Fatalf("agent root package has %d production files, exceeds budget %d", count, maxAgentRootFiles)
	}
}

// One-file pkg directories are allowed only when explicitly reviewed.
func TestPkgOneFileDirectoryAllowlist(t *testing.T) {
	allowlist := map[string]string{
		"cache":      "single cohesive cache manager entrypoint",
		"database":   "single DB connector entrypoint",
		"metrics":    "single metrics collector entrypoint",
		"middleware": "single middleware composition entrypoint",
		"openapi":    "single OpenAPI helper entrypoint",
		"server":     "single server manager entrypoint",
		"telemetry":  "single telemetry setup/shutdown entrypoint",
		"tlsutil":    "single TLS utility entrypoint",
	}

	pkgDirs, err := os.ReadDir("pkg")
	if err != nil {
		t.Fatalf("read pkg dir: %v", err)
	}

	oneFileDirs := map[string]struct{}{}
	for _, d := range pkgDirs {
		if !d.IsDir() {
			continue
		}
		dir := filepath.Join("pkg", d.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		count := 0
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
				count++
			}
		}
		if count == 1 {
			oneFileDirs[d.Name()] = struct{}{}
			if _, ok := allowlist[d.Name()]; !ok {
				t.Fatalf("pkg/%s is a new one-file package without architecture review", d.Name())
			}
		}
	}

	for name := range allowlist {
		if _, ok := oneFileDirs[name]; !ok {
			t.Fatalf("allowlist entry pkg/%s is stale, update architecture guard", name)
		}
	}
}

func TestDependencyDirectionGuards(t *testing.T) {
	type guardRule struct {
		sourcePrefix string
		targetPrefix string
		reason       string
	}

	rules := []guardRule{
		{
			sourcePrefix: "pkg",
			targetPrefix: "api",
			reason:       "infrastructure pkg layer must not depend on API adapter layer",
		},
		{
			sourcePrefix: "pkg",
			targetPrefix: "cmd",
			reason:       "infrastructure pkg layer must not depend on composition root",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "agent",
			reason:       "shared types must stay leaf-level and avoid business dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "api",
			reason:       "shared types must stay leaf-level and avoid adapter dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "cmd",
			reason:       "shared types must stay leaf-level and avoid composition-root dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "config",
			reason:       "shared types must stay leaf-level and avoid runtime config dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "internal",
			reason:       "shared types must stay leaf-level and avoid internal layer dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "llm",
			reason:       "shared types must stay leaf-level and avoid provider/business dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "pkg",
			reason:       "shared types must stay leaf-level and avoid infrastructure dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "rag",
			reason:       "shared types must stay leaf-level and avoid business dependencies",
		},
		{
			sourcePrefix: "types",
			targetPrefix: "workflow",
			reason:       "shared types must stay leaf-level and avoid business dependencies",
		},
	}

	const modulePrefix = "github.com/BaSui01/agentflow/"
	var violations []string
	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(".", path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "/testdata/") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse imports for %s: %w", rel, err)
		}

		for _, imp := range f.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import path for %s: %w", rel, err)
			}
			if !strings.HasPrefix(importPath, modulePrefix) {
				continue
			}

			target := strings.TrimPrefix(importPath, modulePrefix)
			for _, rule := range rules {
				if hasPathPrefix(rel, rule.sourcePrefix) && hasPathPrefix(target, rule.targetPrefix) {
					violations = append(violations, fmt.Sprintf("%s imports %s (%s)", rel, target, rule.reason))
				}
			}
		}
		return nil
	})

	if walkErr != nil {
		t.Fatalf("scan dependency direction guards: %v", walkErr)
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("dependency direction violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestCmdEntrypointImportAllowlist(t *testing.T) {
	allowedImports := map[string]map[string]struct{}{
		"cmd/agentflow/main.go": {
			"github.com/BaSui01/agentflow/internal/app/bootstrap": {},
		},
		"cmd/agentflow/migrate.go": {
			"github.com/BaSui01/agentflow/internal/app/bootstrap": {},
			"github.com/BaSui01/agentflow/pkg/migration":          {},
		},
	}

	var violations []string
	fset := token.NewFileSet()
	const modulePrefix = "github.com/BaSui01/agentflow/"

	for rel, allowlist := range allowedImports {
		path := filepath.FromSlash(rel)
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", rel, err)
		}

		for _, imp := range f.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import path for %s: %v", rel, err)
			}
			if !strings.HasPrefix(importPath, modulePrefix) {
				continue
			}
			if _, ok := allowlist[importPath]; !ok {
				violations = append(violations, fmt.Sprintf("%s imports %s", rel, importPath))
			}
		}
	}

	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("cmd entrypoint import allowlist violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestReadmeCmdAgentflowStructureConsistency(t *testing.T) {
	actualFiles, err := listProductionGoFiles("cmd/agentflow")
	if err != nil {
		t.Fatalf("list production go files: %v", err)
	}

	readmes := []string{"README.md", "README_EN.md"}
	for _, readmePath := range readmes {
		documentedFiles, err := extractCmdAgentflowDocumentedFiles(readmePath)
		if err != nil {
			t.Fatalf("%s: %v", readmePath, err)
		}

		var missing []string
		for name := range actualFiles {
			if _, ok := documentedFiles[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			slices.Sort(missing)
			t.Fatalf("%s is missing cmd/agentflow files: %s", readmePath, strings.Join(missing, ", "))
		}

		var stale []string
		for name := range documentedFiles {
			if _, ok := actualFiles[name]; !ok {
				stale = append(stale, name)
			}
		}
		if len(stale) > 0 {
			slices.Sort(stale)
			t.Fatalf("%s has stale cmd/agentflow files: %s", readmePath, strings.Join(stale, ", "))
		}
	}
}

func listProductionGoFiles(dir string) (map[string]struct{}, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	files := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files[name] = struct{}{}
	}
	return files, nil
}

func extractCmdAgentflowDocumentedFiles(readmePath string) (map[string]struct{}, error) {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		return nil, err
	}
	content := string(raw)
	lines := strings.Split(content, "\n")

	start := -1
	for i, line := range lines {
		if strings.Contains(line, "cmd/agentflow/") {
			start = i
			break
		}
	}
	if start == -1 {
		return nil, fmt.Errorf("cmd/agentflow section not found")
	}

	files := make(map[string]struct{})
	for i := start + 1; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "└── examples/") || strings.HasPrefix(trimmed, "├── examples/") {
			break
		}
		if strings.HasPrefix(trimmed, "```") && len(files) > 0 {
			break
		}
		if !strings.Contains(line, ".go") {
			continue
		}

		for _, field := range strings.Fields(trimmed) {
			if strings.HasSuffix(field, ".go") {
				files[field] = struct{}{}
				break
			}
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no cmd/agentflow .go files parsed from section")
	}
	return files, nil
}

func hasPathPrefix(path, prefix string) bool {
	path = strings.Trim(filepath.ToSlash(path), "/")
	prefix = strings.Trim(filepath.ToSlash(prefix), "/")
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func shouldSkipDir(path string) bool {
	name := filepath.Base(path)
	switch name {
	case ".git", ".snow", ".vscode", "vendor", ".bug":
		return true
	default:
		return false
	}
}

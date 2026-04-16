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
	const maxAgentRootFiles = 26

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
		"jsonschema": "single JSON schema validator entrypoint",
		"metrics":    "single metrics collector entrypoint",
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
			sourcePrefix: "workflow",
			targetPrefix: "agent/persistence",
			reason:       "workflow must not depend on agent persistence implementation",
		},
		{
			sourcePrefix: "rag",
			targetPrefix: "agent",
			reason:       "RAG layer must not depend on agent layer",
		},
		{
			sourcePrefix: "rag",
			targetPrefix: "workflow",
			reason:       "RAG layer must not depend on workflow layer",
		},
		{
			sourcePrefix: "rag",
			targetPrefix: "api",
			reason:       "RAG layer must not depend on API adapter layer",
		},
		{
			sourcePrefix: "rag",
			targetPrefix: "cmd",
			reason:       "RAG layer must not depend on composition root",
		},
		{
			sourcePrefix: "rag",
			targetPrefix: "internal",
			reason:       "RAG layer must not depend on startup-only internal composition support",
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
		{
			sourcePrefix: "llm",
			targetPrefix: "rag",
			reason:       "llm layer must not depend on sibling rag capability layer",
		},
		{
			sourcePrefix: "llm",
			targetPrefix: "agent",
			reason:       "llm layer must not depend on agent layer",
		},
		{
			sourcePrefix: "llm",
			targetPrefix: "workflow",
			reason:       "llm layer must not depend on workflow layer",
		},
		{
			sourcePrefix: "llm",
			targetPrefix: "api",
			reason:       "llm layer must not depend on API adapter layer",
		},
		{
			sourcePrefix: "llm",
			targetPrefix: "cmd",
			reason:       "llm layer must not depend on composition root",
		},
		{
			sourcePrefix: "llm",
			targetPrefix: "internal",
			reason:       "llm layer must not depend on startup-only internal composition support",
		},
		{
			sourcePrefix: "agent",
			targetPrefix: "workflow",
			reason:       "agent layer must not depend upward on workflow orchestrator",
		},
		{
			sourcePrefix: "agent",
			targetPrefix: "api",
			reason:       "agent layer must not depend on API adapter layer",
		},
		{
			sourcePrefix: "agent",
			targetPrefix: "cmd",
			reason:       "agent layer must not depend on composition root",
		},
		{
			sourcePrefix: "agent",
			targetPrefix: "internal",
			reason:       "agent layer must not depend on startup-only internal composition support",
		},
		{
			sourcePrefix: "workflow",
			targetPrefix: "api",
			reason:       "workflow layer must not depend on API adapter layer",
		},
		{
			sourcePrefix: "workflow",
			targetPrefix: "cmd",
			reason:       "workflow layer must not depend on composition root",
		},
		{
			sourcePrefix: "workflow",
			targetPrefix: "internal",
			reason:       "workflow layer must not depend on startup-only internal composition support",
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

func TestLLMComposeImportGuards(t *testing.T) {
	const (
		composeDir       = "llm/runtime/compose"
		configImportPath = "github.com/BaSui01/agentflow/config"
		gormImportPath   = "gorm.io/gorm"
	)

	fset := token.NewFileSet()
	var violations []string

	walkErr := filepath.WalkDir(composeDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, err := filepath.Rel(".", path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse imports for %s: %w", rel, err)
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import path for %s: %w", rel, err)
			}
			if importPath == configImportPath || importPath == gormImportPath {
				violations = append(violations, fmt.Sprintf("%s imports %s", rel, importPath))
			}
		}
		return nil
	})

	if walkErr != nil {
		t.Fatalf("scan llm compose import guards: %v", walkErr)
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("llm compose import violations:\n%s", strings.Join(violations, "\n"))
	}
}

// API handlers must stay at protocol-adapter boundary.
// Allow infra imports only in explicit store adapter files.
func TestAPIHandlerInfraImportGuards(t *testing.T) {
	const (
		handlerDir = "api/handlers"
	)
	disallowedPrefixes := []string{
		"gorm.io/",
		"github.com/BaSui01/agentflow/llm/runtime/router",
		"github.com/BaSui01/agentflow/llm/providers/",
	}
	allowlistFileSuffix := []string{
		"_store.go",
	}

	fset := token.NewFileSet()
	var violations []string

	walkErr := filepath.WalkDir(handlerDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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
		for _, suffix := range allowlistFileSuffix {
			if strings.HasSuffix(rel, suffix) {
				return nil
			}
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("parse imports for %s: %w", rel, err)
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote import path for %s: %w", rel, err)
			}
			for _, prefix := range disallowedPrefixes {
				if strings.HasPrefix(importPath, prefix) {
					violations = append(violations, fmt.Sprintf("%s imports %s", rel, importPath))
				}
			}
		}
		return nil
	})

	if walkErr != nil {
		t.Fatalf("scan api handler infra import guards: %v", walkErr)
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("api handler infra import violations:\n%s", strings.Join(violations, "\n"))
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

func TestReadmeLayerMapAndMatrixConsistency(t *testing.T) {
	type docExpectations struct {
		path             string
		requiredSnippets []string
	}

	expectations := []docExpectations{
		{
			path: "README.md",
			requiredSnippets: []string{
				"### 分层与依赖全图",
				"### 允许依赖 / 禁止依赖矩阵",
				"├── api/                      # 适配层：HTTP/MCP/A2A handler + routes",
				"├── internal/                 # 组合根支撑：启动期 builder / wiring / bridge",
				"├── pkg/                      # 横向基础设施层（不得反向依赖 api/cmd）",
				"├── rag/                      # Layer 2: RAG 检索能力（可被 agent/workflow 复用）",
				"├── workflow/                 # Layer 3: 工作流编排层（位于 agent/rag 之上）",
				"| `workflow/` | `types/`、`llm/`、`agent/`、`rag/`、`pkg/`、`config/` | `api/`、`cmd/`、`internal/`、`agent/persistence` |",
			},
		},
		{
			path: "README_EN.md",
			requiredSnippets: []string{
				"### Full layer map",
				"### Allowed / forbidden dependency matrix",
				"├── api/                      # Adapter layer: HTTP/MCP/A2A handlers + routes",
				"├── internal/                 # Composition-root support: startup builders / bridges",
				"├── pkg/                      # Horizontal infrastructure layer (must not depend on api/cmd)",
				"├── rag/                      # Layer 2: RAG retrieval capability (reused by agent/workflow)",
				"├── workflow/                 # Layer 3: Workflow orchestration (above agent/rag)",
				"| `workflow/` | `types/`, `llm/`, `agent/`, `rag/`, `pkg/`, `config/` | `api/`, `cmd/`, `internal/`, `agent/persistence` |",
			},
		},
	}

	for _, tt := range expectations {
		data, err := os.ReadFile(tt.path)
		if err != nil {
			t.Fatalf("read %s: %v", tt.path, err)
		}
		content := string(data)
		for _, snippet := range tt.requiredSnippets {
			if !strings.Contains(content, snippet) {
				t.Fatalf("%s must contain %q", tt.path, snippet)
			}
		}
	}
}

func TestVendorChatProviderEntryPoints(t *testing.T) {
	type sourceExpectation struct {
		path              string
		requiredSnippets  []string
		forbiddenSnippets []string
	}

	expectations := []sourceExpectation{
		{
			path:             "agentflow.go",
			requiredSnippets: []string{"vendor.NewChatProviderFromConfig("},
		},
		{
			path:             "internal/app/bootstrap/main_provider_registry.go",
			requiredSnippets: []string{"VendorChatProviderFactory{"},
			forbiddenSnippets: []string{
				"NewOpenAIProvider(",
				"NewClaudeProvider(",
				"NewGeminiProvider(",
			},
		},
		{
			path:             "llm/runtime/compose/runtime.go",
			requiredSnippets: []string{"VendorChatProviderFactory{"},
			forbiddenSnippets: []string{
				"NewOpenAIProvider(",
				"NewClaudeProvider(",
				"NewGeminiProvider(",
			},
		},
		{
			path:             "llm/runtime/router/chat_provider_factory.go",
			requiredSnippets: []string{"vendor.NewChatProviderFromConfig("},
		},
	}

	for _, tt := range expectations {
		data, err := os.ReadFile(filepath.FromSlash(tt.path))
		if err != nil {
			t.Fatalf("read %s: %v", tt.path, err)
		}
		src := string(data)
		for _, snippet := range tt.requiredSnippets {
			if !strings.Contains(src, snippet) {
				t.Fatalf("%s must contain %q", tt.path, snippet)
			}
		}
		for _, snippet := range tt.forbiddenSnippets {
			if strings.Contains(src, snippet) {
				t.Fatalf("%s must not contain %q", tt.path, snippet)
			}
		}
	}
}

func TestPublicProviderRoutingDocsUseVendorFactory(t *testing.T) {
	docPaths := []string{
		"README.md",
		"README_EN.md",
		"docs/cn/tutorials/02.Provider配置指南.md",
		"docs/en/tutorials/02.ProviderConfiguration.md",
	}

	for _, path := range docPaths {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := string(data)
		if !strings.Contains(src, "VendorChatProviderFactory") {
			t.Fatalf("%s must document VendorChatProviderFactory for multi-provider routing", path)
		}
		if strings.Contains(src, "NewDefaultProviderFactory()") {
			t.Fatalf("%s must not document legacy NewDefaultProviderFactory() for public multi-provider routing", path)
		}
	}
}

func TestNativeProviderSDKTransportAndToolMappingGuards(t *testing.T) {
	type sourceExpectation struct {
		path              string
		requiredSnippets  []string
		forbiddenSnippets []string
	}

	expectations := []sourceExpectation{
		{
			path: "llm/providers/openai/provider.go",
			requiredSnippets: []string{
				"openaiofficial.NewClient(",
				"providerbase.NewToolCallDeltaAccumulator()",
				"providerbase.BuildOpenAIResponsesToolOutputItem(",
			},
			forbiddenSnippets: []string{
				"http.NewRequest(",
				"http.NewRequestWithContext(",
				"client.Do(",
				"func convertCustomToolFormat(",
				"func normalizeToolType(",
				"func buildToolCallTypeIndex(",
			},
		},
		{
			path: "llm/providers/anthropic/provider.go",
			requiredSnippets: []string{
				"anthropicofficial.NewClient(",
				"providerbase.BuildAnthropicToolResultBlock(",
				"providerbase.NormalizeToolChoice(",
			},
			forbiddenSnippets: []string{
				"http.NewRequest(",
				"http.NewRequestWithContext(",
				"client.Do(",
			},
		},
		{
			path: "llm/providers/gemini/provider.go",
			requiredSnippets: []string{
				"googlegenai.NewClient(",
				"providerbase.BuildGeminiFunctionResponse(",
				"providerbase.NormalizeToolChoice(",
			},
			forbiddenSnippets: []string{
				"func geminiAllowedFunctionNames(",
			},
		},
		{
			path: "llm/providers/gemini/multimodal.go",
			requiredSnippets: []string{
				"client.Tunings.Tune(",
				"client.Tunings.All(",
				"client.Tunings.Get(",
				"client.Tunings.Cancel(",
			},
			forbiddenSnippets: []string{
				"http.NewRequest(",
				"http.NewRequestWithContext(",
				"client.Do(",
				"func postGeminiJSON(",
				"func parseGeminiImageResponse(",
				"func parseGeminiVideoResponse(",
			},
		},
	}

	for _, tt := range expectations {
		data, err := os.ReadFile(filepath.FromSlash(tt.path))
		if err != nil {
			t.Fatalf("read %s: %v", tt.path, err)
		}
		src := string(data)
		for _, snippet := range tt.requiredSnippets {
			if !strings.Contains(src, snippet) {
				t.Fatalf("%s must contain %q", tt.path, snippet)
			}
		}
		for _, snippet := range tt.forbiddenSnippets {
			if strings.Contains(src, snippet) {
				t.Fatalf("%s must not contain %q", tt.path, snippet)
			}
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
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if strings.HasPrefix(trimmed, "├── cmd/agentflow/") || strings.HasPrefix(trimmed, "└── cmd/agentflow/") {
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

// TestAgentPackageExportedErrorStyle ensures agent root package key API files
// do not add bare fmt.Errorf. External API should use agent.NewError / types.Error
// instead. Baseline values are current counts; test fails if count increases.
func TestAgentPackageExportedErrorStyle(t *testing.T) {
	keyFiles := []string{
		"agent/base.go",
		"agent/react.go",
		"agent/integration.go",
		"agent/completion.go",
	}
	maxAllowed := map[string]int{
		"agent/base.go":        0,
		"agent/react.go":       1,
		"agent/integration.go": 0,
		"agent/completion.go":  0,
	}

	for _, rel := range keyFiles {
		content, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		count := strings.Count(string(content), "fmt.Errorf")
		baseline, ok := maxAllowed[rel]
		if !ok {
			continue
		}
		if count > baseline {
			t.Fatalf("file %s has %d fmt.Errorf, exceeds allowed baseline %d — use agent.NewError instead", rel, count, baseline)
		}
	}
}

// TestWorkflowDSLNoMagicStringStepTypes ensures workflow/dsl/ does not use magic
// strings for step type comparisons. Step types must use core.StepType constants.
func TestWorkflowDSLNoMagicStringStepTypes(t *testing.T) {
	disallowed := []string{
		`case "llm":`,
		`case "tool":`,
		`case "agent":`,
		`case "orchestration":`,
		`case "chain":`,
	}

	entries, err := os.ReadDir("workflow/dsl")
	if err != nil {
		t.Fatalf("read workflow/dsl: %v", err)
	}

	var violations []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join("workflow", "dsl", e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			for _, bad := range disallowed {
				if strings.Contains(trimmed, bad) {
					violations = append(violations, fmt.Sprintf("%s:%d: %s (use core.StepType constant)", path, i+1, bad))
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Fatalf("workflow/dsl magic string step type violations:\n%s", strings.Join(violations, "\n"))
	}
}

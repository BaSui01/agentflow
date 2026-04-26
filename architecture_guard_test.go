package agentflow_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestAgentRootPackageFileBudget(t *testing.T) {
	const expectedAgentRootFiles = 0

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

	if count != expectedAgentRootFiles {
		t.Fatalf("agent root package must have %d production files, got %d", expectedAgentRootFiles, count)
	}
}

func TestAgentRootPublicSurfaceBudget(t *testing.T) {
	assertModuleRootNoGoFiles(t, "agent")
}

func TestRAGRootPackageFileBudget(t *testing.T) {
	assertModuleRootNoGoFiles(t, "rag")
}

func TestWorkflowRootPackageFileBudget(t *testing.T) {
	assertModuleRootNoGoFiles(t, "workflow")
}

func TestLLMRootPackageFileBudget(t *testing.T) {
	assertModuleRootNoGoFiles(t, "llm")
}

func assertModuleRootNoGoFiles(t *testing.T, dir string) {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s dir: %v", dir, err)
	}

	var matched []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		matched = append(matched, name)
	}

	if len(matched) > 0 {
		slices.Sort(matched)
		t.Fatalf("%s root package must not expose any Go files, found: %s", dir, strings.Join(matched, ", "))
	}
}

func TestRootLayoutBudget(t *testing.T) {
	const maxTopLevelEntries = 56

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read repo root: %v", err)
	}

	if len(entries) > maxTopLevelEntries {
		t.Fatalf("repo root has %d top-level entries, exceeds budget %d", len(entries), maxTopLevelEntries)
	}
}

func TestPkgOneFileDirectoryAllowlist(t *testing.T) {
	allowlist := map[string]string{
		"cache":      "single cohesive cache manager entrypoint",
		"jsonschema": "single JSON schema validator entrypoint",
		"metrics":    "single metrics collector entrypoint",
		"openapi":    "single OpenAPI helper entrypoint",
		"server":     "single server manager entrypoint",
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
		{sourcePrefix: "pkg", targetPrefix: "api", reason: "infrastructure pkg layer must not depend on API adapter layer"},
		{sourcePrefix: "pkg", targetPrefix: "cmd", reason: "infrastructure pkg layer must not depend on composition root"},
		{sourcePrefix: "workflow", targetPrefix: "agent/persistence", reason: "workflow must not depend on agent persistence implementation"},
		{sourcePrefix: "rag", targetPrefix: "agent", reason: "RAG layer must not depend on agent layer"},
		{sourcePrefix: "rag", targetPrefix: "workflow", reason: "RAG layer must not depend on workflow layer"},
		{sourcePrefix: "rag", targetPrefix: "api", reason: "RAG layer must not depend on API adapter layer"},
		{sourcePrefix: "rag", targetPrefix: "cmd", reason: "RAG layer must not depend on composition root"},
		{sourcePrefix: "rag", targetPrefix: "internal", reason: "RAG layer must not depend on startup-only internal composition support"},
		{sourcePrefix: "types", targetPrefix: "agent", reason: "shared types must stay leaf-level and avoid business dependencies"},
		{sourcePrefix: "types", targetPrefix: "api", reason: "shared types must stay leaf-level and avoid adapter dependencies"},
		{sourcePrefix: "types", targetPrefix: "cmd", reason: "shared types must stay leaf-level and avoid composition-root dependencies"},
		{sourcePrefix: "types", targetPrefix: "config", reason: "shared types must stay leaf-level and avoid runtime config dependencies"},
		{sourcePrefix: "types", targetPrefix: "internal", reason: "shared types must stay leaf-level and avoid internal layer dependencies"},
		{sourcePrefix: "types", targetPrefix: "llm", reason: "shared types must stay leaf-level and avoid provider/business dependencies"},
		{sourcePrefix: "types", targetPrefix: "pkg", reason: "shared types must stay leaf-level and avoid infrastructure dependencies"},
		{sourcePrefix: "types", targetPrefix: "rag", reason: "shared types must stay leaf-level and avoid business dependencies"},
		{sourcePrefix: "types", targetPrefix: "workflow", reason: "shared types must stay leaf-level and avoid business dependencies"},
		{sourcePrefix: "llm", targetPrefix: "rag", reason: "llm layer must not depend on sibling rag capability layer"},
		{sourcePrefix: "llm", targetPrefix: "agent", reason: "llm layer must not depend on agent layer"},
		{sourcePrefix: "llm", targetPrefix: "workflow", reason: "llm layer must not depend on workflow layer"},
		{sourcePrefix: "llm", targetPrefix: "api", reason: "llm layer must not depend on API adapter layer"},
		{sourcePrefix: "llm", targetPrefix: "cmd", reason: "llm layer must not depend on composition root"},
		{sourcePrefix: "llm", targetPrefix: "internal", reason: "llm layer must not depend on startup-only internal composition support"},
		{sourcePrefix: "agent", targetPrefix: "workflow", reason: "agent layer must not depend upward on workflow orchestrator"},
		{sourcePrefix: "agent", targetPrefix: "api", reason: "agent layer must not depend on API adapter layer"},
		{sourcePrefix: "agent", targetPrefix: "cmd", reason: "agent layer must not depend on composition root"},
		{sourcePrefix: "agent", targetPrefix: "internal", reason: "agent layer must not depend on startup-only internal composition support"},
		{sourcePrefix: "workflow", targetPrefix: "api", reason: "workflow layer must not depend on API adapter layer"},
		{sourcePrefix: "workflow", targetPrefix: "cmd", reason: "workflow layer must not depend on composition root"},
		{sourcePrefix: "workflow", targetPrefix: "internal", reason: "workflow layer must not depend on startup-only internal composition support"},
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

func TestAPIHandlerStoreLeakGuards(t *testing.T) {
	var leaked []string

	walkErr := filepath.WalkDir("api/handlers", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "_store.go") {
			return nil
		}
		rel, err := filepath.Rel(".", path)
		if err != nil {
			return err
		}
		leaked = append(leaked, filepath.ToSlash(rel))
		return nil
	})

	if walkErr != nil {
		t.Fatalf("scan api handler store leak guards: %v", walkErr)
	}
	if len(leaked) > 0 {
		slices.Sort(leaked)
		t.Fatalf("api handlers must not own store implementations, found: %s", strings.Join(leaked, ", "))
	}
}

func TestAPIHandlerInfraImportGuards(t *testing.T) {
	disallowedPrefixes := []string{
		"gorm.io/",
		"github.com/BaSui01/agentflow/llm/runtime/router",
		"github.com/BaSui01/agentflow/llm/providers/",
	}

	fset := token.NewFileSet()
	var violations []string

	walkErr := filepath.WalkDir("api/handlers", func(path string, d os.DirEntry, err error) error {
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

func TestCmdHotReloadBootstrapEntryPoints(t *testing.T) {
	data, err := os.ReadFile("cmd/agentflow/server_hotreload.go")
	if err != nil {
		t.Fatalf("read cmd/agentflow/server_hotreload.go: %v", err)
	}
	src := string(data)

	requiredSnippets := []string{
		"bootstrap.ApplyReloadedTextRuntimeBindings(",
		"bootstrap.BuildReloadedResolver(",
		"bootstrap.BuildReloadedWorkflowRuntime(",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(src, snippet) {
			t.Fatalf("cmd/agentflow/server_hotreload.go must contain %q", snippet)
		}
	}

	forbiddenSnippets := []string{
		"handlers.NewChatHandler(",
		"handlers.NewCostHandler(",
		"usecase.NewDefaultWorkflowService(",
		"s.chatHandler.UpdateService(",
		"s.costHandler.UpdateTracker(",
		"s.agentHandler.UpdateService(",
		"s.workflowHandler.UpdateService(",
		"func (s *Server) buildReloadedResolver(",
		"func (s *Server) buildReloadedWorkflowRuntime(",
	}
	for _, snippet := range forbiddenSnippets {
		if strings.Contains(src, snippet) {
			t.Fatalf("cmd/agentflow/server_hotreload.go must not contain %q; delegate handler rebinding to bootstrap seam", snippet)
		}
	}
}

func TestHotReloadDocsImplementationConsistency(t *testing.T) {
	docData, err := os.ReadFile("docs/architecture/启动装配链路与组合根说明.md")
	if err != nil {
		t.Fatalf("read docs/architecture/启动装配链路与组合根说明.md: %v", err)
	}
	doc := string(docData)
	if !strings.Contains(doc, "ApplyReloadedTextRuntimeBindings") {
		t.Fatal("docs/architecture/启动装配链路与组合根说明.md must document ApplyReloadedTextRuntimeBindings")
	}
	if !strings.Contains(doc, "BuildToolingHandlerBundle") {
		t.Fatal("docs/architecture/启动装配链路与组合根说明.md must document BuildToolingHandlerBundle")
	}

	srcData, err := os.ReadFile("internal/app/bootstrap/handler_adapters_builder.go")
	if err != nil {
		t.Fatalf("read internal/app/bootstrap/handler_adapters_builder.go: %v", err)
	}
	src := string(srcData)
	if !strings.Contains(src, "func ApplyReloadedTextRuntimeBindings(") {
		t.Fatal("internal/app/bootstrap/handler_adapters_builder.go must define ApplyReloadedTextRuntimeBindings")
	}
	if !strings.Contains(src, "func BuildToolingHandlerBundle(") {
		t.Fatal("internal/app/bootstrap/handler_adapters_builder.go must define BuildToolingHandlerBundle")
	}
	if !strings.Contains(src, "func BuildReloadedResolver(") {
		t.Fatal("internal/app/bootstrap/handler_adapters_builder.go must define BuildReloadedResolver")
	}
	if !strings.Contains(src, "func BuildReloadedWorkflowRuntime(") {
		t.Fatal("internal/app/bootstrap/handler_adapters_builder.go must define BuildReloadedWorkflowRuntime")
	}

	hotReloadData, err := os.ReadFile("cmd/agentflow/server_hotreload.go")
	if err != nil {
		t.Fatalf("read cmd/agentflow/server_hotreload.go: %v", err)
	}
	if !strings.Contains(string(hotReloadData), "bootstrap.ApplyReloadedTextRuntimeBindings(") {
		t.Fatal("cmd/agentflow/server_hotreload.go must call bootstrap.ApplyReloadedTextRuntimeBindings")
	}

	serveBuilderData, err := os.ReadFile("internal/app/bootstrap/serve_handler_set_builder.go")
	if err != nil {
		t.Fatalf("read internal/app/bootstrap/serve_handler_set_builder.go: %v", err)
	}
	if !strings.Contains(string(serveBuilderData), "BuildToolingHandlerBundle(") {
		t.Fatal("internal/app/bootstrap/serve_handler_set_builder.go must call BuildToolingHandlerBundle")
	}

	if !strings.Contains(doc, "BuildReloadedResolver") {
		t.Fatal("docs/architecture/启动装配链路与组合根说明.md must document BuildReloadedResolver")
	}
	if !strings.Contains(doc, "BuildReloadedWorkflowRuntime") {
		t.Fatal("docs/architecture/启动装配链路与组合根说明.md must document BuildReloadedWorkflowRuntime")
	}
}

func TestUsecaseContractBoundaryGuards(t *testing.T) {
	type fileExpectation struct {
		path              string
		requiredSnippets  []string
		forbiddenSnippets []string
	}

	expectations := []fileExpectation{
		{
			path: "internal/usecase/chat_service.go",
			requiredSnippets: []string{
				"Stream(ctx context.Context, req *ChatRequest) (<-chan ChatStreamEvent, *types.Error)",
			},
			forbiddenSnippets: []string{
				"Stream(ctx context.Context, req *ChatRequest) (<-chan llmcore.UnifiedChunk, *types.Error)",
			},
		},
		{
			path: "internal/usecase/workflow_service.go",
			requiredSnippets: []string{
				"BuildDAGWorkflow(req WorkflowBuildInput) (*WorkflowPlan, string, *types.Error)",
				"Execute(ctx context.Context, wf *WorkflowPlan, input any, streamEmitter WorkflowStreamEmitter, nodeEmitter WorkflowNodeEventEmitter) (any, *types.Error)",
			},
			forbiddenSnippets: []string{
				"BuildDAGWorkflow(req WorkflowBuildInput) (*workflow.DAGWorkflow, string, *types.Error)",
				"Execute(ctx context.Context, wf *workflow.DAGWorkflow, input any, streamEmitter workflow.WorkflowStreamEmitter, nodeEmitter workflowobs.NodeEventEmitter) (any, *types.Error)",
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

	type importGuard struct {
		path             string
		forbiddenImports []string
	}
	importGuards := []importGuard{
		{
			path: "api/handlers/chat.go",
			forbiddenImports: []string{
				"github.com/BaSui01/agentflow/llm/core",
			},
		},
		{
			path: "api/handlers/workflow.go",
			forbiddenImports: []string{
				"github.com/BaSui01/agentflow/workflow/core",
				"github.com/BaSui01/agentflow/workflow/observability",
			},
		},
	}

	fset := token.NewFileSet()
	for _, tt := range importGuards {
		file, err := parser.ParseFile(fset, filepath.FromSlash(tt.path), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", tt.path, err)
		}
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import path for %s: %v", tt.path, err)
			}
			for _, forbidden := range tt.forbiddenImports {
				if importPath == forbidden {
					t.Fatalf("%s must not import %s after usecase contract boundary refactor", tt.path, forbidden)
				}
			}
		}
	}
}

func TestGatewayDirectProviderCallGuards(t *testing.T) {
	protectedPrefixes := []string{
		"workflow",
		"agent/reasoning",
		"agent/structured",
		"agent/evaluation",
		"agent/deliberation",
	}
	allowlistPrefixes := []string{
		"llm/providers",
		"llm/runtime",
		"llm/gateway",
		"internal/app/bootstrap",
	}

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
		if !hasAnyPathPrefix(rel, protectedPrefixes) || hasAnyPathPrefix(rel, allowlistPrefixes) {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse file for %s: %w", rel, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel == nil {
				return true
			}
			if isGatewaySelector(sel.X) {
				return true
			}
			switch sel.Sel.Name {
			case "Completion", "Stream":
				pos := fset.Position(sel.Sel.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d uses direct provider call .%s(...)", rel, pos.Line, sel.Sel.Name))
			}
			return true
		})
		return nil
	})

	if walkErr != nil {
		t.Fatalf("scan gateway direct provider guards: %v", walkErr)
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("business-layer direct provider call violations:\n%s", strings.Join(violations, "\n"))
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
				"├── rag/                      # Layer 2: RAG 检索能力（目录容器；root 无 Go 文件）",
				"├── workflow/                 # Layer 3: 工作流编排层（目录容器；root 无 Go 文件）",
				"| `workflow/` | `types/`\u3001`llm/`\u3001`agent/`\u3001`rag/`\u3001`pkg/`\u3001`config/` | `api/`\u3001`cmd/`\u3001`internal/`\u3001`agent/persistence` |",
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
				"├── rag/                      # Layer 2: RAG retrieval capability (directory-only container; no root Go files)",
				"├── workflow/                 # Layer 3: Workflow orchestration (directory-only container; no root Go files)",
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

func TestAgentUnifiedBuilderEntryPoints(t *testing.T) {
	type sourceExpectation struct {
		path              string
		requiredSnippets  []string
		forbiddenSnippets []string
	}

	expectations := []sourceExpectation{
		{
			path: "agent/runtime/registry_runtime.go",
			requiredSnippets: []string{
				"buildRegistryAgent(",
				"newAgentBuilder(config).",
			},
			forbiddenSnippets: []string{
				"return BuildBaseAgent(config, provider, memory, toolManager, bus, logger, nil), nil",
			},
		},
		{
			path: "agent/team/internal/engines/multiagent/default_modes.go",
			requiredSnippets: []string{
				"newHierarchicalModeBaseAgent(",
				"agentruntime.NewBuilder(gateway, logger).Build(",
			},
			forbiddenSnippets: []string{
				"agent.BuildBaseAgent(types.AgentConfig{",
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

func TestPublicUnifiedEntrypointDocs(t *testing.T) {
	type sourceExpectation struct {
		path              string
		forbiddenSnippets []string
	}

	expectations := []sourceExpectation{
		{path: "README.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "README_EN.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "docs/cn/getting-started/02.框架入口与快速开始.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "docs/cn/tutorials/01.快速开始.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "docs/en/tutorials/01.QuickStart.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "docs/cn/tutorials/03.Agent开发教程.md", forbiddenSnippets: []string{"agent.NewAgentBuilder(", "agent.CreateAgent("}},
		{path: "docs/en/tutorials/03.AgentDevelopment.md", forbiddenSnippets: []string{"agent.NewAgentBuilder(", "agent.CreateAgent("}},
		{path: "docs/cn/tutorials/05.工作流编排.md", forbiddenSnippets: []string{"DAGWorkflow.Execute("}},
		{path: "docs/cn/guides/best-practices.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "README.md", forbiddenSnippets: []string{"`MultiProviderRouter` 与 `ChannelRoutedProvider` 是 `Gateway` 后两个互斥的 routed provider 入口"}},
		{path: "README_EN.md", forbiddenSnippets: []string{"`MultiProviderRouter` and `ChannelRoutedProvider` are the two mutually exclusive routed-provider entries behind `Gateway"}},
		{path: "docs/cn/tutorials/02.Provider配置指南.md", forbiddenSnippets: []string{"`MultiProviderRouter` 与 `ChannelRoutedProvider` 是 `Gateway` 后两个互斥入口"}},
		{path: "docs/en/tutorials/02.ProviderConfiguration.md", forbiddenSnippets: []string{"`MultiProviderRouter` and `ChannelRoutedProvider` are the two mutually exclusive entries behind `Gateway"}},
	}

	for _, tt := range expectations {
		data, err := os.ReadFile(filepath.FromSlash(tt.path))
		if err != nil {
			t.Fatalf("read %s: %v", tt.path, err)
		}
		src := string(data)
		for _, snippet := range tt.forbiddenSnippets {
			if strings.Contains(src, snippet) {
				t.Fatalf("%s must not promote legacy public entry %q; use the unified runtime/facade entry instead", tt.path, snippet)
			}
		}
	}
}

func TestAgentOfficialRuntimeEntrypointDocs(t *testing.T) {
	type sourceExpectation struct {
		path             string
		requiredSnippets []string
	}

	expectations := []sourceExpectation{
		{
			path: "README.md",
			requiredSnippets: []string{
				"`agent/runtime.Builder` 作为 `agent` 子模块 runtime 入口",
				"`github.com/BaSui01/agentflow/agent` 根包已删除",
			},
		},
		{
			path: "README_EN.md",
			requiredSnippets: []string{
				"`agent/runtime.Builder` is the runtime entry for the `agent` submodule",
				"the root package `github.com/BaSui01/agentflow/agent` has been removed",
			},
		},
		{
			path: "docs/cn/getting-started/02.框架入口与快速开始.md",
			requiredSnippets: []string{
				"`agent/runtime.Builder` 仅作为 `agent` 子模块 runtime 入口",
				"`agent/runtime",
			},
		},
		{
			path: "docs/cn/tutorials/01.快速开始.md",
			requiredSnippets: []string{
				"`agent/runtime.Builder` 是 `agent` 子模块 runtime 入口",
			},
		},
		{
			path: "docs/en/tutorials/01.QuickStart.md",
			requiredSnippets: []string{
				"`agent/runtime.Builder` is the runtime entry for the `agent` submodule",
			},
		},
		{
			path: "docs/cn/tutorials/03.Agent开发教程.md",
			requiredSnippets: []string{
				"`agent` 子模块正式 runtime 入口：`agent/runtime.Builder",
			},
		},
		{
			path: "docs/en/tutorials/03.AgentDevelopment.md",
			requiredSnippets: []string{
				"Official runtime entry for the `agent` submodule: `agent/runtime.Builder",
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
				t.Fatalf("%s must contain %q to keep the official runtime entrypoint explicit", tt.path, snippet)
			}
		}
	}
}

func TestOfficialEntrypointDocsConsistency(t *testing.T) {
	type docExpectation struct {
		path                   string
		requiredSnippets       []string
		requiredAdvancedLegacy []string
		forbiddenSnippets      []string
	}

	expectations := []docExpectation{
		{
			path: "README.md",
			requiredSnippets: []string{
				"sdk.New(sdk.Options{",
				"`agent/runtime.Builder` 作为 `agent` 子模块 runtime 入口",
				"`github.com/BaSui01/agentflow/agent` 根包已删除",
			},
			forbiddenSnippets: []string{
				"推荐使用 `agent.NewAgentBuilder",
				"`agent.NewAgentBuilder` 作为正式入口",
				"`agent.NewAgentBuilder",
				"`agent.BuildBaseAgent",
				"`agent.CreateAgent",
			},
		},
		{
			path: "README_EN.md",
			requiredSnippets: []string{
				"sdk.New(sdk.Options{",
				"`agent/runtime.Builder` is the runtime entry for the `agent` submodule",
				"the root package `github.com/BaSui01/agentflow/agent` has been removed",
			},
			forbiddenSnippets: []string{
				"recommend `agent.NewAgentBuilder",
				"`agent.NewAgentBuilder` as the official entrypoint",
				"`agent.NewAgentBuilder",
				"`agent.BuildBaseAgent",
				"`agent.CreateAgent",
			},
		},
		{
			path: "docs/cn/getting-started/02.框架入口与快速开始.md",
			requiredSnippets: []string{
				"sdk.New(sdk.Options{",
				"`agent/runtime.Builder",
				"`agent/runtime",
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
				t.Fatalf("%s must contain %q to keep the official entrypoint set consistent", tt.path, snippet)
			}
		}
		for _, snippet := range tt.requiredAdvancedLegacy {
			if !strings.Contains(src, snippet) {
				t.Fatalf("%s must describe legacy builders as advanced-extension-only and include %q", tt.path, snippet)
			}
		}
		for _, snippet := range tt.forbiddenSnippets {
			if strings.Contains(src, snippet) {
				t.Fatalf("%s must not promote legacy entrypoint phrasing %q", tt.path, snippet)
			}
		}
	}
}

func TestPublicProductSurfaceDocsExamplesConsistency(t *testing.T) {
	type sourceExpectation struct {
		path              string
		requiredSnippets  []string
		forbiddenSnippets []string
	}

	expectations := []sourceExpectation{
		{
			path: "README.md",
			requiredSnippets: []string{
				"**官方多 Agent 门面** - `agent/team",
				"**官方默认** - `ReAct` 作为唯一默认推理/执行主链",
			},
		},
		{
			path: "README_EN.md",
			requiredSnippets: []string{
				"**Official Multi-Agent Facade** - `agent/team",
				"**Official default** - `ReAct` is the only default reasoning/execution chain",
			},
		},
		{
			path: "docs/cn/README.md",
			requiredSnippets: []string{
				"Team 多 Agent 协作",
				"**官方单 Agent 主链**: 默认只走 `react",
			},
			forbiddenSnippets: []string{
				"**推理模式**: ReAct、ReWOO、Plan-Execute、Tree of Thoughts (ToT)",
			},
		},
		{
			path: "docs/en/README.md",
			requiredSnippets: []string{
				"Team Multi-Agent Collaboration",
				"**Official Agent Path**: `react` is the only default runtime path",
			},
			forbiddenSnippets: []string{
				"**Multiple Reasoning Modes**: ReAct, Reflexion, ReWOO, Plan-Execute, Tree of Thoughts (ToT), Dynamic Planner",
			},
		},
		{
			path: "docs/cn/tutorials/03.Agent开发教程.md",
			requiredSnippets: []string{
				"opts.ReasoningExposure = agent.ReasoningExposureAdvanced",
				"## Team（官方多 Agent facade）",
				"## 多 Agent 进阶模式",
				"## Legacy：Crews (CrewAI 风格团队)",
			},
			forbiddenSnippets: []string{
				"github.com/BaSui01/agentflow/agent/hierarchical",
			},
		},
		{
			path: "docs/en/tutorials/03.AgentDevelopment.md",
			requiredSnippets: []string{
				"opts.ReasoningExposure = agent.ReasoningExposureAdvanced",
				"## Team (Official Multi-Agent Facade)",
				"## Advanced Multi-Agent Modes",
				"## Legacy Crews (CrewAI-Style Teams)",
			},
			forbiddenSnippets: []string{
				"github.com/BaSui01/agentflow/agent/hierarchical",
				"github.com/BaSui01/agentflow/agent/memory",
				"github.com/BaSui01/agentflow/agent/guardrails",
			},
		},
		{
			path: "docs/cn/tutorials/08.多Agent协作.md",
			requiredSnippets: []string{
				"`agent/team` 是 AgentFlow 的官方多 Agent facade",
				"## 官方协作模式",
				"## Team 统一抽象（官方）",
			},
			forbiddenSnippets: []string{
				"github.com/BaSui01/agentflow/agent/hierarchical",
			},
		},
		{
			path: "docs/en/tutorials/08.MultiAgentCollaboration.md",
			requiredSnippets: []string{
				"`agent/team` is the official multi-agent facade in AgentFlow",
				"## Official Collaboration Modes",
				"## Legacy Crew Pattern",
			},
			forbiddenSnippets: []string{
				"AgentFlow supports multiple collaboration patterns including hierarchical agents, debate, consensus, pipeline, broadcast, and network modes.",
				"github.com/BaSui01/agentflow/agent/hierarchical",
			},
		},
		{
			path: "examples/08_low_priority_features/README.md",
			requiredSnippets: []string{
				"官方 `agent/team` 多 Agent surface",
				"内部 engine 不作为示例入口",
			},
		},
		{
			path: "examples/09_full_integration/README.md",
			requiredSnippets: []string{
				"legacy 层次化多 Agent",
				"新的多 Agent 接入默认应优先使用 `agent/team",
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
				t.Fatalf("%s must not contain stale public-surface snippet %q", tt.path, snippet)
			}
		}
	}
}

func TestMyAgentFrameworkPublicSurface(t *testing.T) {
	type docExpectation struct {
		path             string
		requiredSnippets []string
	}

	expectations := []docExpectation{
		{
			path: "README.md",
			requiredSnippets: []string{
				"sdk.New(opts).Build(ctx)",
				"`agent/runtime",
				"`agent/team",
				"`workflow/runtime",
			},
		},
		{
			path: "README_EN.md",
			requiredSnippets: []string{
				"sdk.New(opts).Build(ctx)",
				"`agent/runtime",
				"`agent/team",
				"`workflow/runtime",
			},
		},
		{
			path: "AGENTS.md",
			requiredSnippets: []string{
				"`agent/runtime",
				"`agent/team",
				"`workflow/runtime",
				"`internal/usecase/authorization_service.go",
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
				t.Fatalf("%s must contain %q to define the new Agent framework public surface", tt.path, snippet)
			}
		}
	}
}

func TestMyAgentFrameworkNoLegacyPublicEntrypoints(t *testing.T) {
	type docExpectation struct {
		path              string
		forbiddenSnippets []string
	}

	expectations := []docExpectation{
		{
			path: "README.md",
			forbiddenSnippets: []string{
				"`agent/execution/runtime",
				"`agent/collaboration/team",
				"`agent/collaboration/multiagent",
				"`agent/adapters/teamadapter",
			},
		},
		{
			path: "README_EN.md",
			forbiddenSnippets: []string{
				"`agent/execution/runtime",
				"`agent/collaboration/team",
				"`agent/collaboration/multiagent",
				"`agent/adapters/teamadapter",
			},
		},
		{
			path: "AGENTS.md",
			forbiddenSnippets: []string{
				"`agent/execution/runtime",
				"`agent/collaboration/team",
				"`agent/collaboration/multiagent",
				"`agent/adapters/teamadapter",
			},
		},
		{
			path: "docs/cn/README.md",
			forbiddenSnippets: []string{
				"`agent/collaboration/team",
				"`agent/adapters/teamadapter",
			},
		},
		{
			path: "docs/en/getting-started/01.InstallationAndSetup.md",
			forbiddenSnippets: []string{
				"agent/execution/runtime",
				"agent/collaboration/team",
				"agent/collaboration/multiagent",
				"agent/adapters/teamadapter",
			},
		},
	}

	for _, tt := range expectations {
		data, err := os.ReadFile(filepath.FromSlash(tt.path))
		if err != nil {
			t.Fatalf("read %s: %v", tt.path, err)
		}
		src := string(data)
		for _, snippet := range tt.forbiddenSnippets {
			if strings.Contains(src, snippet) {
				t.Fatalf("%s must not contain legacy public entrypoint snippet %q after the hard switch", tt.path, snippet)
			}
		}
	}
}

func TestTeamAdapterIsInternalized(t *testing.T) {
	if _, err := os.Stat(filepath.FromSlash("agent/adapters/teamadapter")); err == nil {
		t.Fatal("agent/adapters/teamadapter must not exist; team adapters belong under agent/team/internal/adapters")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat agent/adapters/teamadapter: %v", err)
	}
	if _, err := os.Stat(filepath.FromSlash("agent/team/internal/adapters/team_adapter.go")); err != nil {
		t.Fatalf("team internal adapter implementation is required: %v", err)
	}
}

func TestUsecaseUsesOfficialTeamExecutionFacade(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("internal/usecase/agent_service.go"))
	if err != nil {
		t.Fatalf("read agent service: %v", err)
	}
	src := string(data)
	for _, forbidden := range []string{
		"GlobalModeRegistry(",
		"NewModeRegistry(",
		"RegisterDefaultModes(",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("internal/usecase must execute multi-agent requests through agent/team facade, found %q", forbidden)
		}
	}
	if !strings.Contains(src, "agentteam.ExecuteAgents(") {
		t.Fatal("internal/usecase must call agentteam.ExecuteAgents for multi-agent execution")
	}
}

func TestNonTeamProductionCodeAvoidsModeRegistryDefaults(t *testing.T) {
	targetDirs := []string{
		"internal/usecase",
		"workflow/steps",
		"agent/runtime/orchestration",
		"internal/app/bootstrap",
	}
	for _, root := range targetDirs {
		err := filepath.WalkDir(filepath.FromSlash(root), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			src := string(data)
			for _, forbidden := range []string{
				"GlobalModeRegistry(",
				"NewModeRegistry(",
				"RegisterDefaultModes(",
			} {
				if strings.Contains(src, forbidden) {
					t.Fatalf("%s must use agent/team facade or ModeExecutor injection instead of %q", filepath.ToSlash(path), forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func TestTutorialsDoNotRecommendTeamInternalPackages(t *testing.T) {
	targetDirs := []string{
		"docs/cn/tutorials",
		"docs/en/tutorials",
		"docs/cn/getting-started",
		"docs/en/getting-started",
	}
	for _, root := range targetDirs {
		err := filepath.WalkDir(filepath.FromSlash(root), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			src := string(data)
			for _, forbidden := range []string{
				"agent/team/internal",
				"hierarchical.NewHierarchicalAgent",
				"collaboration.NewMultiAgentSystem",
				"collaboration.NewMessageHub",
			} {
				if strings.Contains(src, forbidden) {
					t.Fatalf("%s must recommend agent/team facade instead of %q", filepath.ToSlash(path), forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func TestExamplesDoNotImportTeamInternalPackages(t *testing.T) {
	err := filepath.WalkDir("examples", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(data), "agent/team/internal") {
			t.Fatalf("%s must not import or recommend agent/team/internal packages", filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk examples: %v", err)
	}
}

func TestTeamPublicSurfaceDoesNotReexportInternalEngines(t *testing.T) {
	targets := []string{
		"agent/team/team.go",
		"agent/team/execution.go",
		"agent/team/shared_state.go",
	}
	for _, path := range targets {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := string(data)
		for _, forbidden := range []string{
			"type ModeRegistry =",
			"type MultiAgentSystem =",
			"type HierarchicalAgent =",
			"type WorkerPool =",
			"type RolePipeline =",
			"func NewModeRegistry(",
			"func RegisterDefaultModes(",
			"func GlobalModeRegistry(",
			"func NewMultiAgentSystem(",
			"func NewHierarchicalAgent(",
			"func NewTaskCoordinator(",
			"func NewAggregator(",
			"func NewWorkerPool(",
			"func NewRolePipeline(",
			"func NewMessageHub",
		} {
			if strings.Contains(src, forbidden) {
				t.Fatalf("%s must keep internal engine APIs behind the official team facade, found %q", path, forbidden)
			}
		}
	}
}

func TestExamplesAndLivecheckUseOfficialTeamFacade(t *testing.T) {
	targetDirs := []string{
		"examples",
		"scripts/livecheck",
	}
	for _, root := range targetDirs {
		err := filepath.WalkDir(filepath.FromSlash(root), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			src := string(data)
			for _, forbidden := range []string{
				"NewModeRegistry(",
				"RegisterDefaultModes(",
				"GlobalModeRegistry(",
				"DefaultHierarchicalConfig(",
				"NewHierarchicalAgent(",
				"NewTaskCoordinator(",
				"DefaultMultiAgentConfig(",
				"NewMultiAgentSystem(",
				"NewAggregator(",
				"NewWorkerPool(",
				"NewRolePipeline(",
				"NewMessageHub",
			} {
				if strings.Contains(src, forbidden) {
					t.Fatalf("%s must use TeamBuilder, ExecuteAgents, or SupportedExecutionModes instead of %q", filepath.ToSlash(path), forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

func TestNonAgentPackagesUseOfficialAgentFrameworkEntrypoints(t *testing.T) {
	forbidden := []string{
		`"github.com/BaSui01/agentflow/agent/execution/runtime"`,
		`"github.com/BaSui01/agentflow/agent/collaboration/team"`,
		`"github.com/BaSui01/agentflow/agent/collaboration/multiagent"`,
		`"github.com/BaSui01/agentflow/agent/adapters/teamadapter"`,
	}
	allowedPrefixes := []string{
		filepath.FromSlash("agent/"),
		"architecture_guard_test.go",
	}
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "CC-Source" || name == "test_artifacts" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		normalized := filepath.ToSlash(path)
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(normalized, filepath.ToSlash(prefix)) || normalized == prefix {
				return nil
			}
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(data)
		for _, snippet := range forbidden {
			if strings.Contains(src, snippet) {
				t.Fatalf("%s must use agent/runtime or agent/team instead of legacy import %s", normalized, snippet)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk go files: %v", err)
	}
}

func TestAuthorizationDocsUseUnifiedSurface(t *testing.T) {
	paths := []string{
		"AGENTS.md",
		"README.md",
		"README_EN.md",
		"docs/architecture/权限控制系统详细设计-2026-04-24.md",
		"docs/architecture/权限控制系统重构与引入方案-2026-04-24.md",
	}
	for _, path := range paths {
		data, err := os.ReadFile(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := string(data)
		for _, forbidden := range []string{"工具审批系统", "ToolApproval.*唯一"} {
			if strings.Contains(src, forbidden) {
				t.Fatalf("%s must describe authorization through AuthorizationService, found legacy surface %q", path, forbidden)
			}
		}
	}
}

func TestAuthorizationBootstrapFilesDoNotExposeLegacyToolNames(t *testing.T) {
	legacyFiles := []string{
		"internal/app/bootstrap/agent_tool_policy_builder.go",
		"internal/app/bootstrap/agent_tool_approval_builder.go",
	}
	for _, file := range legacyFiles {
		if _, err := os.Stat(filepath.FromSlash(file)); err == nil {
			t.Fatalf("legacy tool-only authorization bootstrap file must be renamed or removed: %s", file)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", file, err)
		}
	}
	currentFiles := []string{
		"internal/app/bootstrap/authorization_builder.go",
		"internal/app/bootstrap/authorization_policy_builder.go",
		"internal/app/bootstrap/authorization_approval_builder.go",
	}
	for _, file := range currentFiles {
		if _, err := os.Stat(filepath.FromSlash(file)); err != nil {
			t.Fatalf("authorization bootstrap file is required: %s: %v", file, err)
		}
	}
}

func TestHostedToolRegistryExecuteStaysBehindAuthorizationAdapters(t *testing.T) {
	allowedFiles := map[string]struct{}{
		filepath.ToSlash("internal/app/bootstrap/agent_tooling_runtime_builder.go"):      {},
		filepath.ToSlash("internal/app/bootstrap/workflow_step_dependencies_builder.go"): {},
	}
	hostedImport := `"github.com/BaSui01/agentflow/agent/integration/hosted"`
	executeSnippets := []string{
		".registry.Execute(",
		".Registry.Execute(",
		"registry.Execute(",
	}

	err := filepath.WalkDir(".", func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		normalized := filepath.ToSlash(path)
		if d.IsDir() {
			switch normalized {
			case ".git", "CC-Source", "docs/claude-code", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(normalized, ".go") || strings.HasSuffix(normalized, "_test.go") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(data)
		if !strings.Contains(src, hostedImport) {
			return nil
		}
		for _, snippet := range executeSnippets {
			if !strings.Contains(src, snippet) {
				continue
			}
			if _, ok := allowedFiles[normalized]; ok {
				return nil
			}
			t.Fatalf("%s directly executes hosted ToolRegistry via %q; route hosted tool execution through AuthorizationService adapters", normalized, snippet)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk go files: %v", err)
	}
}

func TestHighRiskHostedToolConstructorsStayInAuthorizationRuntime(t *testing.T) {
	allowedFiles := map[string]struct{}{
		filepath.ToSlash("internal/app/bootstrap/agent_tooling_runtime_builder.go"): {},
	}
	constructorSnippets := []string{
		"hosted.NewShellTool(",
		"hosted.NewWriteFileTool(",
		"hosted.NewEditFileTool(",
	}

	err := filepath.WalkDir(".", func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		normalized := filepath.ToSlash(path)
		if d.IsDir() {
			switch normalized {
			case ".git", "CC-Source", "docs/claude-code", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(normalized, ".go") || strings.HasSuffix(normalized, "_test.go") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		src := string(data)
		for _, snippet := range constructorSnippets {
			if !strings.Contains(src, snippet) {
				continue
			}
			if _, ok := allowedFiles[normalized]; ok {
				return nil
			}
			t.Fatalf("%s constructs high-risk hosted tool via %q; register shell/file-write tools only in AgentToolingRuntime so execution stays behind AuthorizationService", normalized, snippet)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk go files: %v", err)
	}
}

func TestAuthorizationContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("types/authz.go"))
	if err != nil {
		t.Fatalf("read types/authz.go: %v", err)
	}
	src := string(data)
	requiredSnippets := []string{
		"type Principal struct",
		"type AuthorizationRequest struct",
		"type AuthorizationDecision struct",
		"type PrincipalKind string",
		"type ResourceKind string",
		"type ActionKind string",
		"type DecisionKind string",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(src, snippet) {
			t.Fatalf("types/authz.go must contain %q to expose zero-dependency authorization contracts", snippet)
		}
	}
}

func TestWorkflowTeamBoundary(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("workflow/steps/orchestration.go"))
	if err != nil {
		t.Fatalf("read workflow/steps/orchestration.go: %v", err)
	}
	src := string(data)

	if !strings.Contains(src, `"github.com/BaSui01/agentflow/agent/team"`) {
		t.Fatal("workflow/steps/orchestration.go must depend on the team-owned mode engine seam")
	}
}

func TestAgentExecutionOptionsArchitectureGuards(t *testing.T) {
	t.Run("loop_executor_uses_resolved_control_options", func(t *testing.T) {
		data, err := os.ReadFile("agent/runtime/agent_builder.go")
		if err != nil {
			t.Fatalf("read agent/runtime/agent_builder.go: %v", err)
		}
		src := string(data)
		start := strings.Index(src, "// Merged from loop_executor.go.")
		end := strings.Index(src, "// Merged from loop_executor_runtime.go.")
		if start == -1 || end == -1 || end <= start {
			t.Fatal("agent/builder.go must keep explicit merged loop executor section markers")
		}
		src = src[start:end]
		for _, needle := range []string{
			"ResolveRunConfig(",
			"DisablePlannerEnabled(",
			"topLevelLoopBudget(",
		} {
			if strings.Contains(src, needle) {
				t.Fatalf("agent/runtime/agent_builder.go must not depend on legacy control fallback %q", needle)
			}
		}
	})

	t.Run("chat_request_construction_stays_in_adapter", func(t *testing.T) {
		requestData, err := os.ReadFile("agent/runtime/request_runtime.go")
		if err != nil {
			t.Fatalf("read agent/runtime/request_runtime.go: %v", err)
		}
		if strings.Contains(string(requestData), "ChatRequest{") {
			t.Fatal("agent/runtime/request_runtime.go must not construct ChatRequest directly; use ChatRequestAdapter")
		}

		adapterData, err := os.ReadFile("agent/adapters/chat.go")
		if err != nil {
			t.Fatalf("read agent/adapters/chat.go: %v", err)
		}
		adapterSrc := string(adapterData)
		if !strings.Contains(adapterSrc, "ChatRequest{") {
			t.Fatal("agent/adapters/chat.go must remain the primary ChatRequest construction surface")
		}
		if !strings.Contains(adapterSrc, "func toolChoiceToRequestValue(choice *types.ToolChoice) any") {
			t.Fatal("agent/adapters/chat.go must remain the adapter boundary that lowers ToolChoice into provider request payloads")
		}

		if _, err := os.Stat("agent/chat_request_adapter.go"); err == nil {
			t.Fatal("agent/chat_request_adapter.go must be removed once ChatRequestAdapter moves under agent/adapters")
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat agent/chat_request_adapter.go: %v", err)
		}
	})

	t.Run("tool_choice_any_stays_out_of_agent_runtime_surface", func(t *testing.T) {
		runConfigData, err := os.ReadFile("types/run_config.go")
		if err != nil {
			t.Fatalf("read types/run_config.go: %v", err)
		}
		if !strings.Contains(string(runConfigData), "ParseToolChoiceString(") {
			t.Fatal("types/run_config.go must normalize legacy tool_choice strings into types.ToolChoice before execution")
		}

		entries, err := os.ReadDir("agent")
		if err != nil {
			t.Fatalf("read agent dir: %v", err)
		}

		var violations []string
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join("agent", name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			src := string(data)
			if strings.Contains(src, "ToolChoice any") || strings.Contains(src, "ToolChoice interface{}") {
				violations = append(violations, path)
			}
		}
		if len(violations) > 0 {
			slices.Sort(violations)
			t.Fatalf("agent root package must not expose ToolChoice as any outside provider DTOs:\n%s", strings.Join(violations, "\n"))
		}
	})
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

func hasAnyPathPrefix(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if hasPathPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func isGatewaySelector(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.Ident:
		return strings.HasSuffix(v.Name, "Gateway")
	case *ast.SelectorExpr:
		return v.Sel != nil && strings.HasSuffix(v.Sel.Name, "Gateway")
	default:
		return false
	}
}

func shouldSkipDir(path string) bool {
	switch filepath.Base(path) {
	case ".git", ".snow", ".vscode", "vendor", ".bug":
		return true
	default:
		return false
	}
}

func TestServiceBackedHandlersUseBaseHandler(t *testing.T) {
	type handlerExpectation struct {
		file             string
		mustContainBase  bool
		forbiddenSnippet string
	}

	expectations := []handlerExpectation{
		{file: "api/handlers/agent.go", mustContainBase: true},
		{file: "api/handlers/apikey.go", mustContainBase: true},
		{file: "api/handlers/authorization_audit.go", mustContainBase: true},
		{file: "api/handlers/chat.go", mustContainBase: true},
		{file: "api/handlers/cost.go", mustContainBase: true},
		{file: "api/handlers/multimodal.go", mustContainBase: true},
		{file: "api/handlers/rag.go", mustContainBase: true},
		{file: "api/handlers/tool_approval.go", mustContainBase: true},
		{file: "api/handlers/tool_provider.go", mustContainBase: true},
		{file: "api/handlers/tool_registry.go", mustContainBase: true},
		{file: "api/handlers/workflow.go", mustContainBase: true},
		{file: "api/handlers/health.go", mustContainBase: false},
		{file: "api/handlers/protocol.go", mustContainBase: false},
	}

	for _, tt := range expectations {
		data, err := os.ReadFile(filepath.FromSlash(tt.file))
		if err != nil {
			t.Fatalf("read %s: %v", tt.file, err)
		}
		src := string(data)
		hasBase := strings.Contains(src, "BaseHandler[")
		if tt.mustContainBase && !hasBase {
			t.Fatalf("%s must embed BaseHandler for service-backed hot-reload", tt.file)
		}
		if !tt.mustContainBase && hasBase {
			t.Fatalf("%s must stay outside BaseHandler exception policy", tt.file)
		}
	}
}

func TestExecutionModeCatalogSingleSource(t *testing.T) {
	allowedDefinitions := map[string][]string{
		"agent/team/execution.go": {
			"func SupportedExecutionModes()",
			"func IsSupportedExecutionMode(",
			"func NormalizeExecutionMode(",
		},
		"internal/usecase/agent_execution_modes.go": {
			"func normalizedExecutionMode(",
			"func SupportedExecutionModes()",
			"func IsSupportedExecutionMode(",
		},
	}

	definitionSnippets := []string{
		"func SupportedExecutionModes()",
		"func IsSupportedExecutionMode(",
		"func NormalizeExecutionMode(",
		"func normalizedExecutionMode(",
	}

	var violations []string
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
		if !hasPathPrefix(rel, "agent") && !hasPathPrefix(rel, "internal") && !hasPathPrefix(rel, "workflow") && !hasPathPrefix(rel, "api") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		for _, snippet := range definitionSnippets {
			if !strings.Contains(src, snippet) {
				continue
			}
			allowed := false
			for _, allowSnippet := range allowedDefinitions[rel] {
				if allowSnippet == snippet {
					allowed = true
					break
				}
			}
			if !allowed {
				violations = append(violations, rel+" defines "+snippet)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan execution mode catalog sources: %v", walkErr)
	}
	if len(violations) > 0 {
		slices.Sort(violations)
		t.Fatalf("execution mode catalog definitions must stay in agent/team facade or usecase thin wrappers:\n%s", strings.Join(violations, "\n"))
	}
}

func TestRuntimeFileBudget(t *testing.T) {
	const maxLines = 1500
	var violations []string

	runtimeDir := filepath.FromSlash("agent/runtime")
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		t.Fatalf("read runtime dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(runtimeDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lines := strings.Count(string(data), "\n") + 1
		if lines > maxLines {
			violations = append(violations, fmt.Sprintf("%s: %d lines (max %d)", filepath.ToSlash(path), lines, maxLines))
		}
	}
	if len(violations) > 0 {
		t.Fatalf("runtime files exceed line budget:\n%s", strings.Join(violations, "\n"))
	}
}

func TestUsecaseDoesNotImportMultiagent(t *testing.T) {
	forbidden := "agent/team/internal/engines/multiagent"
	var violations []string

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
		rel, _ := filepath.Rel(".", path)
		rel = filepath.ToSlash(rel)
		if !hasPathPrefix(rel, "internal/usecase") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "\""+forbidden+"\"") {
			violations = append(violations, rel+" imports "+forbidden)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan usecase imports: %v", walkErr)
	}
	if len(violations) > 0 {
		t.Fatalf("usecase must not import multiagent internal package:\n%s", strings.Join(violations, "\n"))
	}
}

func TestAPIHandlerDoesNotImportNonBuilderRuntime(t *testing.T) {
	forbiddenPatterns := []string{
		"agent/runtime\".",
	}
	var violations []string

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
		rel, _ := filepath.Rel(".", path)
		rel = filepath.ToSlash(rel)
		if !hasPathPrefix(rel, "api") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		for _, pattern := range forbiddenPatterns {
			if strings.Contains(src, pattern) {
				violations = append(violations, rel+" contains "+pattern)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("scan api imports: %v", walkErr)
	}
	if len(violations) > 0 {
		t.Fatalf("api handlers must not import agent/runtime non-Builder constructors:\n%s", strings.Join(violations, "\n"))
	}
}

func TestNoForbiddenTopLevelPackages(t *testing.T) {
	forbidden := []string{"crew", "flow", "graph", "society", "pipeline"}
	for _, pkg := range forbidden {
		if _, err := os.Stat(pkg); err == nil {
			t.Fatalf("forbidden top-level package '%s' exists — use sdk/runtime/team/workflow as official entrypoints", pkg)
		}
	}
}

func TestWorkflowTeamBoundaryGuard(t *testing.T) {
	workflowFiles := map[string]bool{}
	walkErr := filepath.WalkDir(filepath.FromSlash("workflow/runtime"), func(path string, d os.DirEntry, err error) error {
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
		workflowFiles[path] = true
		return nil
	})
	if walkErr != nil {
		t.Skipf("workflow/runtime not found: %v", walkErr)
	}

	forbiddenPatterns := []string{"Handoff", "Supervisor", "Swarm"}
	var violations []string
	for path := range workflowFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		src := string(data)
		for _, pattern := range forbiddenPatterns {
			if strings.Contains(src, pattern) {
				rel, _ := filepath.Rel(".", path)
				violations = append(violations, filepath.ToSlash(rel)+" contains autonomous pattern "+pattern)
			}
		}
	}
	if len(violations) > 0 {
		t.Fatalf("workflow/runtime must not contain autonomous collaboration patterns (belongs in agent/team):\n%s", strings.Join(violations, "\n"))
	}
}

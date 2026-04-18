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
	const maxAgentRootFiles = 29

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

func TestPkgOneFileDirectoryAllowlist(t *testing.T) {
	allowlist := map[string]string{
		"cache":      "single cohesive cache manager entrypoint",
		"database":   "single DB connector entrypoint",
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

func TestAPIHandlerInfraImportGuards(t *testing.T) {
	disallowedPrefixes := []string{
		"gorm.io/",
		"github.com/BaSui01/agentflow/llm/runtime/router",
		"github.com/BaSui01/agentflow/llm/providers/",
	}
	allowlistFileSuffix := []string{"_store.go"}

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

func TestAgentUnifiedBuilderEntryPoints(t *testing.T) {
	type sourceExpectation struct {
		path              string
		requiredSnippets  []string
		forbiddenSnippets []string
	}

	expectations := []sourceExpectation{
		{
			path: "agent/registry.go",
			requiredSnippets: []string{
				"buildRegistryAgent(",
				"newAgentBuilder(config).",
			},
			forbiddenSnippets: []string{
				"return NewBaseAgent(config, provider, memory, toolManager, bus, logger, nil), nil",
			},
		},
		{
			path: "agent/multiagent/default_modes.go",
			requiredSnippets: []string{
				"newHierarchicalModeBaseAgent(",
				"agentruntime.NewBuilder(gateway, logger).Build(",
			},
			forbiddenSnippets: []string{
				"agent.NewBaseAgent(types.AgentConfig{",
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
		{path: "docs/getting_started.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "docs/cn/tutorials/01.快速开始.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "docs/en/tutorials/01.QuickStart.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "docs/cn/tutorials/03.Agent开发教程.md", forbiddenSnippets: []string{"agent.NewAgentBuilder(", "agent.CreateAgent("}},
		{path: "docs/en/tutorials/03.AgentDevelopment.md", forbiddenSnippets: []string{"agent.NewAgentBuilder(", "agent.CreateAgent("}},
		{path: "docs/cn/tutorials/05.工作流编排.md", forbiddenSnippets: []string{"DAGWorkflow.Execute("}},
		{path: "docs/cn/guides/best-practices.md", forbiddenSnippets: []string{"agent.NewAgentBuilder("}},
		{path: "README.md", forbiddenSnippets: []string{"`MultiProviderRouter` 与 `ChannelRoutedProvider` 是 `Gateway` 后两个互斥的 routed provider 入口"}},
		{path: "README_EN.md", forbiddenSnippets: []string{"`MultiProviderRouter` and `ChannelRoutedProvider` are the two mutually exclusive routed-provider entries behind `Gateway`"}},
		{path: "docs/cn/tutorials/02.Provider配置指南.md", forbiddenSnippets: []string{"`MultiProviderRouter` 与 `ChannelRoutedProvider` 是 `Gateway` 后两个互斥入口"}},
		{path: "docs/en/tutorials/02.ProviderConfiguration.md", forbiddenSnippets: []string{"`MultiProviderRouter` and `ChannelRoutedProvider` are the two mutually exclusive entries behind `Gateway`"}},
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
				"**官方多 Agent 门面** - `agent/team`",
				"**官方默认** - `ReAct` 作为唯一默认推理/执行主链",
			},
		},
		{
			path: "README_EN.md",
			requiredSnippets: []string{
				"**Official Multi-Agent Facade** - `agent/team`",
				"**Official default** - `ReAct` is the only default reasoning/execution chain",
			},
		},
		{
			path: "docs/cn/README.md",
			requiredSnippets: []string{
				"Team 与 Legacy 多 Agent 协作",
				"**官方单 Agent 主链**: 默认只走 `react`",
			},
			forbiddenSnippets: []string{
				"**推理模式**: ReAct、ReWOO、Plan-Execute、Tree of Thoughts (ToT)",
			},
		},
		{
			path: "docs/en/README.md",
			requiredSnippets: []string{
				"Team & Legacy Multi-Agent Collaboration",
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
				"## Legacy：多 Agent 协作",
			},
		},
		{
			path: "docs/en/tutorials/03.AgentDevelopment.md",
			requiredSnippets: []string{
				"opts.ReasoningExposure = agent.ReasoningExposureAdvanced",
				"## Team (Official Multi-Agent Facade)",
				"## Legacy Multi-Agent Collaboration",
			},
		},
		{
			path: "docs/cn/tutorials/08.多Agent协作.md",
			requiredSnippets: []string{
				"`agent/team` 是 AgentFlow 的官方多 Agent facade",
				"## Legacy：多 Agent 系统",
			},
		},
		{
			path: "docs/en/tutorials/08.MultiAgentCollaboration.md",
			requiredSnippets: []string{
				"`agent/team` is the official multi-agent facade in AgentFlow",
				"## Legacy Multi-Agent System",
			},
			forbiddenSnippets: []string{
				"AgentFlow supports multiple collaboration patterns including hierarchical agents, debate, consensus, pipeline, broadcast, and network modes.",
			},
		},
		{
			path: "examples/08_low_priority_features/README.md",
			requiredSnippets: []string{
				"legacy 多 Agent surface",
				"新的多 Agent 接入默认应优先使用 `agent/team`",
			},
		},
		{
			path: "examples/09_full_integration/README.md",
			requiredSnippets: []string{
				"legacy 层次化多 Agent",
				"新的多 Agent 接入默认应优先使用 `agent/team`",
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

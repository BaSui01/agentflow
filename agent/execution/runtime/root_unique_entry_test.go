package runtime

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentUniqueEntry 验证 agent 根包不再暴露并行的 New* Agent 构造面；
// 正式 runtime 入口由 agent/execution/runtime.Builder 承担。
// 这是 TDD Red 阶段的测试，预期失败直到重构完成
func TestAgentUniqueEntry(t *testing.T) {
	agentRootDir := "."

	// 解析根目录所有 .go 文件（排除测试文件）
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, agentRootDir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && strings.HasSuffix(fi.Name(), ".go")
	}, 0)
	if err != nil {
		t.Fatalf("解析 agent 根目录失败: %v", err)
	}

	// 收集所有“Agent 构造面”导出的构造函数。
	// 这里故意不把 NewError / NewPromptBundle 等辅助构造计入，
	// 只关注真正会创建 Agent / BaseAgent / AgentBuilder 一类入口的函数。
	var constructors []string
	for _, pkg := range pkgs {
		for fileName, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				if fn, ok := n.(*ast.FuncDecl); ok {
					funcName := fn.Name.Name
					if fn.Name.IsExported() && strings.HasPrefix(funcName, "New") && isAgentConstructionSurface(fn) {
						// 排除测试辅助函数
						if !strings.Contains(filepath.Base(fileName), "_test") {
							constructors = append(constructors, funcName)
						}
					}
				}
				return true
			})
		}
	}

	// 允许的唯一入口：NewBuilder（或 AgentBuilder 的构造函数）
	allowedConstructors := map[string]bool{
		"NewBuilder":      true, // 如果存在
		"NewAgentBuilder": true, // 测试辅助可能用到
	}

	var violations []string
	for _, ctor := range constructors {
		if !allowedConstructors[ctor] {
			violations = append(violations, ctor)
		}
	}

	if len(violations) > 0 {
		t.Errorf("agent 根目录存在 %d 个非唯一入口的构造函数，违反单一入口原则:\n%v\n期望：只保留 Builder 作为唯一对外入口",
			len(violations), violations)
	}
}

func isAgentConstructionSurface(fn *ast.FuncDecl) bool {
	if fn.Type == nil || fn.Type.Results == nil {
		return false
	}
	for _, field := range fn.Type.Results.List {
		typeName := exprString(field.Type)
		switch typeName {
		case "*BaseAgent", "BaseAgent", "Agent", "*AgentBuilder", "AgentBuilder", "*Builder", "Builder":
			return true
		}
	}
	return false
}

func exprString(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	case *ast.SelectorExpr:
		return fmt.Sprintf("%s.%s", exprString(v.X), v.Sel.Name)
	default:
		return ""
	}
}



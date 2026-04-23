package runtime

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAgentDependencyDirection 验证 agent 包内部依赖方向是否符合分层架构
// 这是 TDD Red 阶段的测试，预期失败直到重构完成
func TestAgentDependencyDirection(t *testing.T) {
	agentDir := "."

	// 定义依赖方向规则（层级从低到高）
	// 低层不能依赖高层
	layerHierarchy := map[string]int{
		"core":          1, // 最底层
		"capabilities":  2,
		"execution":     3,
		"persistence":   3, // 与 execution 同级
		"collaboration": 4,
		"integration":   5,
		"observability": 5, // 与 integration 同级
		"adapters":      5, // 与 integration 同级
	}

	// 检查每个层级目录
	for layer, level := range layerHierarchy {
		layerPath := filepath.Join(agentDir, layer)
		if _, err := os.Stat(layerPath); os.IsNotExist(err) {
			// 目录不存在，跳过（架构守卫测试会捕获）
			continue
		}

		// 解析该层所有 Go 文件
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, layerPath, nil, parser.ImportsOnly)
		if err != nil {
			t.Logf("警告：解析 %s 失败: %v", layer, err)
			continue
		}

		// 检查导入路径
		for _, pkg := range pkgs {
			for _, file := range pkg.Files {
				for _, imp := range file.Imports {
					importPath := strings.Trim(imp.Path.Value, `"`)

					// 检查是否导入了更高层的包
					if strings.Contains(importPath, "github.com/BaSui01/agentflow/agent/") {
						importedLayer := extractLayer(importPath)
						if importedLayer != "" {
							if importedLevel, ok := layerHierarchy[importedLayer]; ok {
								if importedLevel > level {
									t.Errorf("依赖方向违规：%s/ (层级 %d) 不应依赖 %s/ (层级 %d)",
										layer, level, importedLayer, importedLevel)
								}
							}
						}
					}
				}
			}
		}
	}
}

// extractLayer 从导入路径中提取层级名称
// 例如：github.com/BaSui01/agentflow/agent/capabilities/memory -> capabilities
func extractLayer(importPath string) string {
	parts := strings.Split(importPath, "/agent/")
	if len(parts) < 2 {
		return ""
	}
	subPath := parts[1]
	layerParts := strings.Split(subPath, "/")
	if len(layerParts) > 0 {
		return layerParts[0]
	}
	return ""
}



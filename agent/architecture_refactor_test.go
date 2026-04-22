package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAgentArchitectureLayering 验证 agent 包的 8 层架构是否已建立
// 这是 TDD Red 阶段的测试，预期失败直到重构完成
func TestAgentArchitectureLayering(t *testing.T) {
	agentDir := "."

	// 期望的 8 个顶层目录
	expectedLayers := []string{
		"core",
		"capabilities",
		"execution",
		"collaboration",
		"persistence",
		"integration",
		"observability",
		"adapters",
	}

	for _, layer := range expectedLayers {
		layerPath := filepath.Join(agentDir, layer)
		if _, err := os.Stat(layerPath); os.IsNotExist(err) {
			t.Errorf("目标架构层 %s/ 不存在，重构尚未完成", layer)
		}
	}

	// 验证旧目录已删除（部分示例）
	oldDirs := []string{
		"memorycore",
		"guardcore",
		"deliberation",
		"teamadapter",
		"crews",
		"discovery",
		"longrunning",
	}

	for _, oldDir := range oldDirs {
		oldPath := filepath.Join(agentDir, oldDir)
		if _, err := os.Stat(oldPath); err == nil {
			t.Errorf("旧目录 %s/ 仍然存在，应该已被合并删除", oldDir)
		}
	}
}

// TestAgentRootFilesBudget 验证根目录文件数预算（目标 ≤ 9 个）
func TestAgentRootFilesBudget(t *testing.T) {
	agentDir := "."

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		t.Fatalf("无法读取 agent 目录: %v", err)
	}

	var goFileCount int
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" && !isTestFile(entry.Name()) {
			goFileCount++
		}
	}

	const maxRootFiles = 9
	if goFileCount > maxRootFiles {
		t.Errorf("agent 根目录有 %d 个 .go 文件，超过预算 %d（目标：只保留最小公开面）", goFileCount, maxRootFiles)
	}
}

func isTestFile(name string) bool {
	return filepath.Ext(name) == ".go" &&
		(len(name) > 8 && name[len(name)-8:] == "_test.go")
}

package agent

import (
	"bufio"
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

	// 验证旧目录已删除（包含 Phase-5 清零的 21 个目录做防回潮）
	oldDirs := []string{
		"memorycore",
		"guardcore",
		"deliberation",
		"teamadapter",
		"crews",
		"discovery",
		"longrunning",
		// Phase-5 清零的空目录
		"artifacts",
		"context",
		"conversation",
		"declarative",
		"deployment",
		"evaluation",
		"handoff",
		"hitl",
		"hosted",
		"k8s",
		"lsp",
		"multiagent",
		"orchestration",
		"planner",
		"reasoning",
		"runtime",
		"skills",
		"streaming",
		"structured",
		"voice",
		// Phase-5 合并到 agent/core/ 的内部目录
		"internalcore",
	}

	for _, oldDir := range oldDirs {
		oldPath := filepath.Join(agentDir, oldDir)
		if _, err := os.Stat(oldPath); err == nil {
			t.Errorf("旧目录 %s/ 仍然存在，应该已被合并删除", oldDir)
		}
	}
}

// TestAgentTopLevelDirectoryAllowlist 验证 agent/ 下顶层子目录严格等于 8 层 allowlist
func TestAgentTopLevelDirectoryAllowlist(t *testing.T) {
	agentDir := "."

	allowlist := map[string]bool{
		"adapters":      true,
		"capabilities":  true,
		"collaboration": true,
		"core":          true,
		"execution":     true,
		"integration":   true,
		"observability": true,
		"persistence":   true,
	}

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		t.Fatalf("无法读取 agent 目录: %v", err)
	}

	found := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		found[name] = true
		if !allowlist[name] {
			t.Errorf("顶层目录 %s 不在允许集合内，应该已被清理", name)
		}
	}

	for name := range allowlist {
		if !found[name] {
			t.Errorf("允许集合中的目录 %s 不存在，8 层架构不完整", name)
		}
	}
}

// TestAgentRootFilesBudget 验证根目录文件数预算（目标 ≤ 5 个）
func TestAgentRootFilesBudget(t *testing.T) {
	agentDir := "."

	allowedRootFiles := map[string]bool{
		"base.go":       true,
		"builder.go":    true,
		"interfaces.go": true,
		"registry.go":   true,
		"request.go":    true,
	}

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		t.Fatalf("无法读取 agent 目录: %v", err)
	}

	var goFileCount int
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" && !isTestFile(entry.Name()) {
			goFileCount++
			if !allowedRootFiles[entry.Name()] {
				t.Errorf("根目录文件 %s 未在允许集合内，应该已下沉到 8 层目录", entry.Name())
			}
		}
	}

	const maxRootFiles = 5
	if goFileCount > maxRootFiles {
		t.Errorf("agent 根目录有 %d 个 .go 文件，超过预算 %d（目标：只保留最小公开面）", goFileCount, maxRootFiles)
	}
}

// TestAgentRootLinesOfCodeBudget 验证根目录非测试 Go 文件总行数预算（目标 < 2000）
func TestAgentRootLinesOfCodeBudget(t *testing.T) {
	agentDir := "."

	entries, err := os.ReadDir(agentDir)
	if err != nil {
		t.Fatalf("无法读取 agent 目录: %v", err)
	}

	var total int
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || isTestFile(entry.Name()) {
			continue
		}
		f, err := os.Open(filepath.Join(agentDir, entry.Name()))
		if err != nil {
			t.Fatalf("无法打开文件 %s: %v", entry.Name(), err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			total++
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			t.Fatalf("读取文件 %s 出错: %v", entry.Name(), err)
		}
	}

	const maxRootLOC = 2000
	if total >= maxRootLOC {
		t.Errorf("agent 根目录非测试 go 行数 %d，超过预算 %d，需要继续按职责簇下沉", total, maxRootLOC)
	}
}

func isTestFile(name string) bool {
	return filepath.Ext(name) == ".go" &&
		(len(name) > 8 && name[len(name)-8:] == "_test.go")
}

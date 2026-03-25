package agent

import (
	"os"
	"strings"
	"testing"
)

func TestClosedLoopPlanDoc_RetainsRegressionGuardItems(t *testing.T) {
	content, err := os.ReadFile("../docs/重构计划/闭环Agent完善计划-2026-03-25.md")
	if err != nil {
		t.Fatalf("read plan doc: %v", err)
	}

	required := []string{
		"`BaseAgent.Execute(...)` 不得直接回退为单步 ReAct 主链",
		"`reasoningRegistry` 已注入时，默认主链必须消费模式选择结果",
		"`Plan/Observe` 必须出现在默认闭环阶段流转中",
		"`multiagent.ModeLoop` 不得成为单 Agent 默认执行旁路",
		"增加默认闭环执行链测试：简单任务、工具任务、失败重规划、反思修正、人工升级",
		"SSE 新增统一 `status` 事件并稳定输出 `reasoning_mode_selected/completion_judge_decision/loop_stopped`",
	}

	doc := string(content)
	for _, needle := range required {
		if !strings.Contains(doc, needle) {
			t.Fatalf("plan doc missing regression guard item: %s", needle)
		}
	}
}

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

	requireAnyTopic(t, doc, "validation stage", "validate", "validation stage", "validation/acceptance", "闭环验收")
	requireAnyTopic(t, doc, "acceptance criteria", "acceptance criteria", "acceptance_criteria", "验收标准")
	requireAnyTopic(t, doc, "tool verification", "tool verification", "tool_verification", "工具验证")
	requireAnyTopic(t, doc, "task-level loop budget", "max_loop_iterations", "top-level loop budget", "任务级 loop budget")
	requireAnyTopic(t, doc, "non-empty output is not enough", "non-empty output", "非空输出", "不能直接 solved")
}

func requireAnyTopic(t *testing.T, doc string, topic string, needles ...string) {
	t.Helper()

	docLower := strings.ToLower(doc)
	for _, needle := range needles {
		if strings.Contains(docLower, strings.ToLower(needle)) {
			return
		}
	}

	t.Fatalf("plan doc missing regression guard topic %q (expected one of %v)", topic, needles)
}

import unittest
from pathlib import Path


SKILL_DIR = Path(__file__).resolve().parents[1]
SKILL_FILE = SKILL_DIR / "SKILL.md"
TEMPLATE_FILE = SKILL_DIR / "references" / "重构计划模板.md"
OPENAI_FILE = SKILL_DIR / "agents" / "openai.yaml"
RUN_GUARD_FILE = SKILL_DIR / "scripts" / "run_guard.ps1"


class RefactorPlanGovernanceSkillContractTest(unittest.TestCase):
    def test_skill_requires_tdd_and_strict_completion(self) -> None:
        text = SKILL_FILE.read_text(encoding="utf-8")

        self.assertRegex(text, r"TDD|测试驱动")
        self.assertIn("先写失败测试", text)
        self.assertIn("验证命令", text)
        self.assertIn("通过标准", text)
        self.assertRegex(text, r"全部通过.*才允许输出[“\"]?完成|通过.*才算完成")

    def test_template_contains_red_green_refactor_and_binary_dod(self) -> None:
        text = TEMPLATE_FILE.read_text(encoding="utf-8")

        self.assertIn("测试策略（TDD）", text)
        self.assertIn("失败测试", text)
        self.assertRegex(text, r"转绿|绿灯")
        self.assertIn("重构", text)
        self.assertGreaterEqual(text.count("验证命令："), 3)
        self.assertGreaterEqual(text.count("通过标准："), 3)

    def test_agent_prompt_surfaces_new_requirements(self) -> None:
        text = OPENAI_FILE.read_text(encoding="utf-8")

        self.assertRegex(text, r"TDD|测试驱动")
        self.assertIn("通过标准", text)
        self.assertIn("RequireTDD", text)

    def test_wrapper_exposes_strict_guard_switches(self) -> None:
        text = RUN_GUARD_FILE.read_text(encoding="utf-8")

        self.assertIn("RequireTDD", text)
        self.assertIn("RequireVerifiableCompletion", text)
        self.assertIn("--require-tdd", text)
        self.assertIn("--require-verifiable-completion", text)


if __name__ == "__main__":
    unittest.main()

import importlib.util
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).resolve().parents[1] / "refactor_plan_guard.py"
SPEC = importlib.util.spec_from_file_location("refactor_plan_guard", MODULE_PATH)
GUARD = importlib.util.module_from_spec(SPEC)
assert SPEC and SPEC.loader
sys.modules[SPEC.name] = GUARD
SPEC.loader.exec_module(GUARD)


STRICT_REQUIREMENTS = GUARD.PlanRequirements(
    require_tdd=True,
    require_verifiable_completion=True,
)


def write_plan(root: Path, name: str, content: str) -> Path:
    path = root / name
    path.write_text(textwrap.dedent(content).strip() + "\n", encoding="utf-8")
    return path


class RefactorPlanGuardStrictModeTest(unittest.TestCase):
    def test_rejects_missing_tdd_section_when_required(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = write_plan(
                Path(tmp),
                "plan.md",
                """
                ## 0. 执行状态总览

                - [x] 概览完成

                ## 4. 执行计划（Phase）

                - [x] 主链路改造（验证命令：`go test ./agent`; 通过标准：退出码为 0）

                ## 6. 完成定义（DoD）

                - [x] 验收完成（验证命令：`python scripts/refactor_plan_guard.py gate`; 通过标准：退出码为 0）
                """,
            )

            stats = GUARD.collect_stats(path, requirements=STRICT_REQUIREMENTS)

            self.assertTrue(any("测试策略（TDD）" in err for err in stats.errors))

    def test_rejects_missing_red_green_refactor_details(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = write_plan(
                Path(tmp),
                "plan.md",
                """
                ## 0. 执行状态总览

                - [x] 概览完成

                ## 3. 测试策略（TDD）

                - [x] 补测试（验证命令：`go test ./agent`; 通过标准：退出码为 0）

                ## 4. 执行计划（Phase）

                - [x] 主链路改造（验证命令：`go test ./agent`; 通过标准：退出码为 0）

                ## 6. 完成定义（DoD）

                - [x] 验收完成（验证命令：`python scripts/refactor_plan_guard.py gate`; 通过标准：退出码为 0）
                """,
            )

            stats = GUARD.collect_stats(path, requirements=STRICT_REQUIREMENTS)

            self.assertTrue(any("先写失败测试" in err for err in stats.errors))
            self.assertTrue(any("测试转绿" in err or "转绿" in err for err in stats.errors))
            self.assertTrue(any("重构" in err for err in stats.errors))

    def test_rejects_missing_verifiable_completion_lines(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = write_plan(
                Path(tmp),
                "plan.md",
                """
                ## 0. 执行状态总览

                - [x] 概览完成

                ## 3. 测试策略（TDD）

                - [x] 先写失败测试并确认红灯（验证命令：`go test ./agent -run TestLoop`; 通过标准：新增测试先失败）
                - [x] 采用最小实现让测试转绿（验证命令：`go test ./agent -run TestLoop`; 通过标准：目标测试转绿）
                - [x] 完成重构并执行回归验证（验证命令：`go test ./agent`; 通过标准：相关测试全部通过）

                ## 4. 执行计划（Phase）

                - [x] 主链路改造

                ## 6. 完成定义（DoD）

                - [x] 验收完成
                """,
            )

            stats = GUARD.collect_stats(path, requirements=STRICT_REQUIREMENTS)

            self.assertTrue(any("执行计划" in err and "验证命令" in err for err in stats.errors))
            self.assertTrue(any("完成定义" in err and "通过标准" in err for err in stats.errors))

    def test_accepts_plan_that_meets_strict_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = write_plan(
                Path(tmp),
                "plan.md",
                """
                ## 0. 执行状态总览

                - [x] 概览完成

                ## 3. 测试策略（TDD）

                - [x] 先写失败测试并确认红灯（验证命令：`go test ./agent -run TestLoop`; 通过标准：新增测试先失败，且失败原因与待修问题直接对应）
                - [x] 采用最小实现让测试转绿（验证命令：`go test ./agent -run TestLoop`; 通过标准：目标测试转绿，且未引入兼容分支）
                - [x] 完成重构并执行回归验证（验证命令：`go test ./agent`; 通过标准：相关测试全部通过，且旧实现已删除）

                ## 4. 执行计划（Phase）

                - [x] 主链路改造（验证命令：`go test ./agent`; 通过标准：主链路测试通过）

                ## 6. 完成定义（DoD）

                - [x] 验收完成（验证命令：`python scripts/refactor_plan_guard.py gate --target plan.md --require-tdd --require-verifiable-completion`; 通过标准：退出码为 0，且允许停止/收尾）
                """,
            )

            stats = GUARD.collect_stats(path, requirements=STRICT_REQUIREMENTS)

            self.assertEqual([], stats.errors)


if __name__ == "__main__":
    unittest.main()

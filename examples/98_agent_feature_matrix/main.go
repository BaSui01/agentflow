package main

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	runTimeout       = 4 * time.Minute
	maxOutputPreview = 24000
)

type exampleSpec struct {
	Key          string
	Category     string
	Name         string
	RelPath      string
	Description  string
	Tags         []string
	WarnPatterns []string
}

type exampleResult struct {
	Spec       exampleSpec
	Status     string
	Duration   time.Duration
	ExitCode   int
	Summary    string
	Highlights []string
	Output     string
	Command    string
}

func main() {
	root := repoRoot()
	specs := buildSpecs()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║   AgentFlow Agent Feature Matrix                            ║")
	fmt.Println("║   中文可视化总览：当前项目 Agent 示例能力全量重测          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	var results []exampleResult
	for _, spec := range specs {
		result := runExample(root, spec)
		results = append(results, result)
		printResult(result)
	}

	raw := buildTextReport(results)
	reportPath, rawPath := writeArtifacts(root, raw, results)

	fmt.Println("\n=== 汇总 ===")
	printSummary(results)
	fmt.Printf("  文本报告: %s\n", rawPath)
	fmt.Printf("  HTML 报告: %s\n", reportPath)
}

func buildSpecs() []exampleSpec {
	return []exampleSpec{
		{
			Key:         "01_simple_chat",
			Category:    "单 Agent 基础",
			Name:        "简单对话",
			RelPath:     "examples/01_simple_chat",
			Description: "最基础的模型问答链路",
			Tags:        []string{"LLM", "单Agent", "对话"},
		},
		{
			Key:         "02_streaming",
			Category:    "单 Agent 基础",
			Name:        "流式输出",
			RelPath:     "examples/02_streaming",
			Description: "验证增量流式返回",
			Tags:        []string{"LLM", "Streaming"},
		},
		{
			Key:         "03_tool_use",
			Category:    "单 Agent 基础",
			Name:        "工具调用",
			RelPath:     "examples/03_tool_use",
			Description: "验证 function calling / ReAct 基础链路",
			Tags:        []string{"Tools", "ReAct"},
		},
		{
			Key:         "04_custom_agent",
			Category:    "单 Agent 基础",
			Name:        "自定义 Agent",
			RelPath:     "examples/04_custom_agent",
			Description: "验证 AgentBuilder 与自定义配置",
			Tags:        []string{"AgentBuilder", "配置"},
			WarnPatterns: []string{
				"unauthorized",
				"failed:",
			},
		},
		{
			Key:         "05_workflow",
			Category:    "工作流编排",
			Name:        "基础工作流",
			RelPath:     "examples/05_workflow",
			Description: "验证工作流节点编排",
			Tags:        []string{"Workflow", "DAG"},
		},
		{
			Key:         "06_advanced_features",
			Category:    "Agent 增强",
			Name:        "高级单 Agent",
			RelPath:     "examples/06_advanced_features",
			Description: "反射、动态工具选择、提示词增强/模板",
			Tags:        []string{"Reflection", "Prompt", "ToolSelect"},
			WarnPatterns: []string{
				"reflection execution failed",
				"llm ranking failed",
			},
		},
		{
			Key:         "07_mid_priority_features",
			Category:    "多 Agent 与托管工具",
			Name:        "中优先级能力",
			RelPath:     "examples/07_mid_priority_features",
			Description: "Hosted Tools、Handoff、Crews、Conversation、Tracing",
			Tags:        []string{"HostedTools", "Handoff", "Crews", "Tracing"},
			WarnPatterns: []string{
				"web search execution failed",
				"web search results: 0",
			},
		},
		{
			Key:         "08_low_priority_features",
			Category:    "多 Agent 与托管工具",
			Name:        "低优先级能力",
			RelPath:     "examples/08_low_priority_features",
			Description: "层次化、多 Agent 协作、可观测性",
			Tags:        []string{"Hierarchy", "Collaboration", "Observability"},
		},
		{
			Key:         "09_full_integration",
			Category:    "集成场景",
			Name:        "全功能集成",
			RelPath:     "examples/09_full_integration",
			Description: "增强单 Agent + 多 Agent + 生产配置",
			Tags:        []string{"Integration", "Production"},
			WarnPatterns: []string{
				"[agent_not_ready]",
			},
		},
		{
			Key:         "12_complete_rag_system",
			Category:    "知识与检索",
			Name:        "完整 RAG",
			RelPath:     "examples/12_complete_rag_system",
			Description: "向量检索、混合检索、语义缓存、上下文压缩",
			Tags:        []string{"RAG", "Retrieval", "Cache"},
		},
		{
			Key:         "14_guardrails",
			Category:    "安全与护栏",
			Name:        "Guardrails",
			RelPath:     "examples/14_guardrails",
			Description: "PII 检测、注入检测、验证器链",
			Tags:        []string{"PII", "Injection", "Validators"},
		},
		{
			Key:         "15_structured_output",
			Category:    "结构化能力",
			Name:        "结构化输出",
			RelPath:     "examples/15_structured_output",
			Description: "Schema 生成、校验、手工构建",
			Tags:        []string{"JSONSchema", "Structured"},
		},
		{
			Key:         "16_a2a_protocol",
			Category:    "协议与互联",
			Name:        "A2A 协议",
			RelPath:     "examples/16_a2a_protocol",
			Description: "Agent Card、消息协议、Client/Server",
			Tags:        []string{"A2A", "Protocol"},
		},
		{
			Key:         "17_high_priority_features",
			Category:    "工作流编排",
			Name:        "高优先级能力",
			RelPath:     "examples/17_high_priority_features",
			Description: "Artifacts、HITL、OpenAPI Tools、Deployment、Visual Builder",
			Tags:        []string{"Artifacts", "HITL", "OpenAPI", "VisualBuilder"},
			WarnPatterns: []string{
				"step dependency not configured",
			},
		},
		{
			Key:         "18_advanced_agent_features",
			Category:    "Agent 增强",
			Name:        "高级 Agent 总成",
			RelPath:     "examples/18_advanced_agent_features",
			Description: "联邦编排、深思模式、长时运行、技能注册表等",
			Tags:        []string{"Federation", "Deliberation", "LongRunning", "Skills"},
			WarnPatterns: []string{
				"setup canary db error",
				"go-sqlite3 requires cgo to work",
			},
		},
		{
			Key:         "19_2026_features",
			Category:    "前沿能力",
			Name:        "2026 扩展能力",
			RelPath:     "examples/19_2026_features",
			Description: "Layered Memory、GraphRAG、Shadow AI、Infra Managers",
			Tags:        []string{"Memory", "GraphRAG", "ShadowAI"},
			WarnPatterns: []string{
				"redis",
				"sqlite cgo",
				"skip",
			},
		},
		{
			Key:         "21_research_workflow",
			Category:    "工作流编排",
			Name:        "研究工作流模板",
			RelPath:     "examples/21_research_workflow",
			Description: "研究自动化 DAG 模板",
			Tags:        []string{"Research", "DAG", "Template"},
		},
	}
}

func runExample(root string, spec exampleSpec) exampleResult {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	command := fmt.Sprintf("go run ./%s", filepath.ToSlash(spec.RelPath))
	cmd := exec.CommandContext(ctx, "go", "run", "./"+filepath.ToSlash(spec.RelPath))
	cmd.Dir = root
	cmd.Env = os.Environ()

	start := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)
	output := trimOutput(normalizeOutput(string(out)))
	status, exitCode, summary := classifyResult(ctx, err, output, spec)

	return exampleResult{
		Spec:       spec,
		Status:     status,
		Duration:   duration,
		ExitCode:   exitCode,
		Summary:    summary,
		Highlights: extractHighlights(output),
		Output:     output,
		Command:    command,
	}
}

func classifyResult(ctx context.Context, err error, output string, spec exampleSpec) (string, int, string) {
	if ctx.Err() == context.DeadlineExceeded {
		return "FAIL", -1, "执行超时"
	}

	lower := strings.ToLower(output)
	exitCode := 0
	if err != nil {
		exitCode = 1
		var exitErr *exec.ExitError
		if ok := errorAs(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		}
		if hasLinePrefix(output, "panic:") {
			return "FAIL", exitCode, lineWithPrefix(output, "panic:")
		}
		return "FAIL", exitCode, firstUsefulLine(output)
	}

	for _, pattern := range spec.WarnPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return "WARN", exitCode, firstMatchingLine(output, pattern)
		}
	}

	return "PASS", exitCode, pickSummary(output)
}

func printResult(result exampleResult) {
	icon := map[string]string{
		"PASS": "PASS",
		"WARN": "WARN",
		"FAIL": "FAIL",
	}[result.Status]
	fmt.Printf("\n[%s] %s\n", icon, result.Spec.Name)
	fmt.Printf("  分类: %s\n", result.Spec.Category)
	fmt.Printf("  命令: %s\n", result.Command)
	fmt.Printf("  用时: %s\n", result.Duration.Round(time.Millisecond))
	fmt.Printf("  摘要: %s\n", result.Summary)
	for _, line := range result.Highlights {
		fmt.Printf("  亮点: %s\n", line)
	}
}

func printSummary(results []exampleResult) {
	passCount := 0
	warnCount := 0
	failCount := 0
	for _, result := range results {
		switch result.Status {
		case "PASS":
			passCount++
		case "WARN":
			warnCount++
		case "FAIL":
			failCount++
		}
	}

	fmt.Printf("  总数=%d PASS=%d WARN=%d FAIL=%d\n", len(results), passCount, warnCount, failCount)
	for _, result := range results {
		fmt.Printf("  %-18s %-12s %8s  %s\n",
			result.Spec.Category,
			result.Spec.Name,
			result.Status,
			result.Summary,
		)
	}
}

func buildTextReport(results []exampleResult) string {
	var b strings.Builder
	b.WriteString("AgentFlow Agent Feature Matrix\n")
	b.WriteString(fmt.Sprintf("重测时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05 -07:00")))

	for _, result := range results {
		b.WriteString(fmt.Sprintf("=== %s / %s ===\n", result.Spec.Category, result.Spec.Name))
		b.WriteString(fmt.Sprintf("路径: %s\n", result.Spec.RelPath))
		b.WriteString(fmt.Sprintf("命令: %s\n", result.Command))
		b.WriteString(fmt.Sprintf("状态: %s\n", result.Status))
		b.WriteString(fmt.Sprintf("耗时: %s\n", result.Duration.Round(time.Millisecond)))
		b.WriteString(fmt.Sprintf("摘要: %s\n", result.Summary))
		if len(result.Spec.Tags) > 0 {
			b.WriteString(fmt.Sprintf("标签: %s\n", strings.Join(result.Spec.Tags, " / ")))
		}
		if len(result.Highlights) > 0 {
			for _, line := range result.Highlights {
				b.WriteString(fmt.Sprintf("亮点: %s\n", line))
			}
		}
		b.WriteString("原始输出:\n")
		b.WriteString(result.Output)
		if !strings.HasSuffix(result.Output, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("=== Summary ===\n")
	passCount := 0
	warnCount := 0
	failCount := 0
	for _, result := range results {
		switch result.Status {
		case "PASS":
			passCount++
		case "WARN":
			warnCount++
		case "FAIL":
			failCount++
		}
	}
	b.WriteString(fmt.Sprintf("Total=%d PASS=%d WARN=%d FAIL=%d\n", len(results), passCount, warnCount, failCount))
	for _, result := range results {
		b.WriteString(fmt.Sprintf("  %-18s %-12s %-4s %8v  %s\n",
			result.Spec.Category,
			result.Spec.Name,
			result.Status,
			result.Duration.Round(time.Millisecond),
			result.Summary,
		))
	}
	b.WriteString("\n=== Web 搜索现状与对标 ===\n")
	b.WriteString(buildWebSearchBenchmarkText())

	return b.String()
}

func writeArtifacts(root, raw string, results []exampleResult) (string, string) {
	outDir := filepath.Join(root, "examples", "98_agent_feature_matrix")
	_ = os.MkdirAll(outDir, 0o755)

	rawPath := filepath.Join(outDir, "latest_run.txt")
	htmlPath := filepath.Join(outDir, "latest_report.html")

	_ = os.WriteFile(rawPath, []byte(raw), 0o644)
	_ = os.WriteFile(htmlPath, []byte(buildHTMLReport(results, raw)), 0o644)

	return htmlPath, rawPath
}

func buildHTMLReport(results []exampleResult, raw string) string {
	passCount := 0
	warnCount := 0
	failCount := 0
	categoryStats := map[string][3]int{}
	for _, result := range results {
		stats := categoryStats[result.Spec.Category]
		switch result.Status {
		case "PASS":
			passCount++
			stats[0]++
		case "WARN":
			warnCount++
			stats[1]++
		case "FAIL":
			failCount++
			stats[2]++
		}
		categoryStats[result.Spec.Category] = stats
	}

	var cards bytes.Buffer
	lastCategory := ""
	for _, result := range results {
		if result.Spec.Category != lastCategory {
			if lastCategory != "" {
				cards.WriteString("</div></section>")
			}
			lastCategory = result.Spec.Category
			stats := categoryStats[result.Spec.Category]
			cards.WriteString(fmt.Sprintf(
				`<section class="panel"><div class="section-head"><h2>%s</h2><p>PASS %d / WARN %d / FAIL %d</p></div><div class="grid">`,
				html.EscapeString(result.Spec.Category),
				stats[0],
				stats[1],
				stats[2],
			))
		}

		statusClass := strings.ToLower(result.Status)
		cards.WriteString(`<article class="card">`)
		cards.WriteString(fmt.Sprintf(`<div class="badge %s">%s</div>`, statusClass, html.EscapeString(result.Status)))
		cards.WriteString(fmt.Sprintf(`<h3>%s</h3>`, html.EscapeString(result.Spec.Name)))
		cards.WriteString(fmt.Sprintf(`<p class="desc">%s</p>`, html.EscapeString(result.Spec.Description)))
		cards.WriteString(fmt.Sprintf(`<p class="meta">路径：%s</p>`, html.EscapeString(result.Spec.RelPath)))
		cards.WriteString(fmt.Sprintf(`<p class="meta">耗时：%s</p>`, html.EscapeString(result.Duration.Round(time.Millisecond).String())))
		cards.WriteString(fmt.Sprintf(`<p class="summary">%s</p>`, html.EscapeString(result.Summary)))
		if len(result.Spec.Tags) > 0 {
			cards.WriteString(`<div class="tags">`)
			for _, tag := range result.Spec.Tags {
				cards.WriteString(fmt.Sprintf(`<span>%s</span>`, html.EscapeString(tag)))
			}
			cards.WriteString(`</div>`)
		}
		if len(result.Highlights) > 0 {
			cards.WriteString(`<ul>`)
			for _, line := range result.Highlights {
				cards.WriteString(fmt.Sprintf(`<li>%s</li>`, html.EscapeString(line)))
			}
			cards.WriteString(`</ul>`)
		}
		cards.WriteString(`</article>`)
	}
	if cards.Len() > 0 {
		cards.WriteString("</div></section>")
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>AgentFlow Agent Feature Matrix</title>
  <style>
    :root {
      --bg: #efe9dd;
      --panel: #fffdf7;
      --ink: #1d2a2e;
      --muted: #5a6a70;
      --line: #d8ccbc;
      --hero-a: #18354a;
      --hero-b: #4f7f67;
      --pass: #2f7f4f;
      --warn: #b37b12;
      --fail: #b24437;
    }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: "Segoe UI", "Microsoft YaHei", sans-serif; background: radial-gradient(circle at top, #f8f3ea, var(--bg)); color: var(--ink); }
    .wrap { max-width: 1280px; margin: 0 auto; padding: 24px 18px 48px; }
    .hero { background: linear-gradient(135deg, var(--hero-a), var(--hero-b)); color: #fff; border-radius: 22px; padding: 28px; box-shadow: 0 16px 40px rgba(24,53,74,.18); }
    h1 { margin: 0 0 8px; font-size: 34px; }
    .sub { margin: 0; color: rgba(255,255,255,.84); line-height: 1.6; }
    .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; margin-top: 18px; }
    .stat { background: rgba(255,255,255,.12); border-radius: 16px; padding: 14px 16px; }
    .label { font-size: 12px; letter-spacing: .08em; text-transform: uppercase; opacity: .82; }
    .value { font-size: 30px; font-weight: 700; }
    .panel { margin-top: 18px; background: var(--panel); border: 1px solid var(--line); border-radius: 18px; padding: 18px; box-shadow: 0 12px 28px rgba(29,42,46,.06); }
    .section-head { display: flex; justify-content: space-between; align-items: baseline; gap: 12px; }
    .section-head h2 { margin: 0 0 12px; font-size: 22px; }
    .section-head p { margin: 0; color: var(--muted); }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 14px; }
    .card { position: relative; border: 1px solid var(--line); border-radius: 16px; padding: 18px; background: linear-gradient(180deg, #fffefb, #f8f3ea); min-height: 240px; }
    .card h3 { margin: 10px 0 8px; font-size: 20px; }
    .desc, .meta, .summary { margin: 8px 0; line-height: 1.6; }
    .meta { color: var(--muted); font-size: 13px; }
    .summary { font-weight: 600; }
    .badge { display: inline-block; padding: 6px 10px; border-radius: 999px; font-size: 12px; font-weight: 700; letter-spacing: .06em; }
    .badge.pass { background: rgba(47,127,79,.12); color: var(--pass); }
    .badge.warn { background: rgba(179,123,18,.12); color: var(--warn); }
    .badge.fail { background: rgba(178,68,55,.12); color: var(--fail); }
    .tags { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
    .tags span { background: #ede4d8; color: #3e4b50; border-radius: 999px; padding: 4px 10px; font-size: 12px; }
    ul { margin: 12px 0 0; padding-left: 18px; }
    li { margin: 6px 0; line-height: 1.5; }
    pre { margin: 0; white-space: pre-wrap; word-break: break-word; font-size: 13px; line-height: 1.55; background: #fbf7ef; border-radius: 14px; padding: 16px; border: 1px solid #e6dac8; }
  </style>
</head>
<body>
  <div class="wrap">
    <section class="hero">
      <h1>AgentFlow Agent Feature Matrix</h1>
      <p class="sub">重测时间：%s。这个页面聚焦项目当前可运行的 Agent 示例能力，中文展示每个示例对应的功能面、运行状态、关键亮点与当前阻塞项。</p>
      <div class="stats">
        <div class="stat"><div class="label">Total</div><div class="value">%d</div></div>
        <div class="stat"><div class="label">Pass</div><div class="value">%d</div></div>
        <div class="stat"><div class="label">Warn</div><div class="value">%d</div></div>
        <div class="stat"><div class="label">Fail</div><div class="value">%d</div></div>
      </div>
    </section>
    <section class="panel">
      <div class="section-head">
        <h2>读图说明</h2>
        <p>PASS=可运行且主链通过，WARN=可运行但有局部缺口，FAIL=当前硬失败</p>
      </div>
      <ul>
        <li>这份矩阵覆盖单 Agent、多 Agent、工作流、RAG、安全护栏、A2A、结构化输出、2026 扩展能力等当前仓内示例。</li>
        <li>Provider 级能力单独看 examples/99_capability_matrix/latest_report.html；本页主要回答“AgentFlow 目前有哪些 Agent 功能真的能跑起来”。</li>
        <li>中优先级示例里的 hosted web search 已切到项目内置的 provider-backed 实现，不再使用假 endpoint。</li>
      </ul>
    </section>
    <section class="panel">
      <div class="section-head">
        <h2>Web 搜索现状与对标</h2>
        <p>基于 2026-03-22 复核的官方文档与当前仓内实现</p>
      </div>
      %s
    </section>
    %s
    <section class="panel">
      <div class="section-head">
        <h2>原始输出</h2>
        <p>便于核对每个示例的真实 stdout/stderr</p>
      </div>
      <pre>%s</pre>
    </section>
  </div>
</body>
</html>`,
		time.Now().Format("2006-01-02 15:04:05 -07:00"),
		len(results),
		passCount,
		warnCount,
		failCount,
		buildWebSearchBenchmarkHTML(),
		cards.String(),
		html.EscapeString(raw),
	)
}

func normalizeOutput(out string) string {
	out = strings.ReplaceAll(out, "\r\n", "\n")
	return strings.TrimSpace(out)
}

func trimOutput(out string) string {
	if len(out) <= maxOutputPreview {
		return out
	}

	headLen := maxOutputPreview / 2
	tailLen := maxOutputPreview / 2
	return out[:headLen] + fmt.Sprintf("\n\n...[已截断 %d 字符]...\n\n", len(out)-maxOutputPreview) + out[len(out)-tailLen:]
}

func extractHighlights(output string) []string {
	lines := nonEmptyLines(output)
	var highlights []string
	for _, line := range lines {
		if isHeadingLine(line) {
			continue
		}
		if strings.HasPrefix(line, "Error") || strings.HasPrefix(strings.ToLower(line), "panic:") {
			highlights = append(highlights, line)
			continue
		}
		if strings.Contains(line, ":") || strings.Contains(line, "->") {
			highlights = append(highlights, line)
		}
		if len(highlights) == 4 {
			break
		}
	}
	if len(highlights) == 0 && len(lines) > 0 {
		highlights = append(highlights, lines[0])
	}
	return highlights
}

func pickSummary(output string) string {
	lines := nonEmptyLines(output)
	for _, line := range lines {
		if isHeadingLine(line) {
			continue
		}
		if strings.Contains(line, "=== All") || strings.Contains(line, "Demo Complete") {
			continue
		}
		return line
	}
	return "运行完成"
}

func firstUsefulLine(output string) string {
	lines := nonEmptyLines(output)
	priorities := []string{"panic:", "failed", "error", "unauthorized", "not in ready state"}
	for _, keyword := range priorities {
		for _, line := range lines {
			if strings.Contains(strings.ToLower(line), keyword) {
				return line
			}
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "exit status") {
			continue
		}
		return line
	}
	return "执行失败"
}

func firstMatchingLine(output, pattern string) string {
	lines := nonEmptyLines(output)
	for _, line := range lines {
		if strings.Contains(strings.ToLower(line), strings.ToLower(pattern)) {
			return line
		}
	}
	return firstUsefulLine(output)
}

func lineWithPrefix(output, prefix string) string {
	lines := nonEmptyLines(output)
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
			return line
		}
	}
	return firstUsefulLine(output)
}

func nonEmptyLines(output string) []string {
	raw := strings.Split(output, "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}

func isHeadingLine(line string) bool {
	return strings.HasPrefix(line, "===") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "╔") || strings.HasPrefix(line, "║") || strings.HasPrefix(line, "╚")
}

func buildWebSearchBenchmarkText() string {
	return strings.Join([]string{
		"- 当前仓内已具备两层 web 搜索能力：一层是 provider-native（OpenAI Responses、Anthropic Messages、Gemini grounding），一层是 hosted provider-backed（tavily/firecrawl/duckduckgo/searxng）。",
		"- 本次已把 examples/07_mid_priority_features 从假 endpoint 改成真实 provider-backed hosted web_search；无 API key 时默认走 duckduckgo。",
		"- OpenAI 官方文档显示 Responses API 的 web_search 支持 citations、sources 列表、domain filtering、external_web_access，并在 reasoning 模型下支持 open_page / find_in_page。文档: https://platform.openai.com/docs/guides/tools-web-search",
		"- Anthropic 官方文档显示 web_search_20250305 支持 max_uses、allowed_domains / blocked_domains、approximate user_location，Claude 最终回答会带来源引用。文档: https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool",
		"- Google Vertex AI 官方文档显示 Gemini grounding 不仅支持 Google Search，还支持 Vertex AI Search 与 external search API，并允许组合多种 grounding source。文档: https://docs.cloud.google.com/vertex-ai/generative-ai/docs/grounding/overview",
		"- LangChain 官方文档显示常见路线是 Tavily/SerpAPI 等外部搜索工具，外加网页浏览或抓取工具；Google 集成还直接暴露 Gemini native tools。文档: https://docs.langchain.com/langsmith/test-react-agent-pytest , https://docs.langchain.com/oss/javascript/integrations/tools/google , https://docs.langchain.com/oss/javascript/integrations/tools/webbrowser/",
		"- 结合本仓代码与这些官方能力做判断：AgentFlow 现在“接口面”已经不缺，主要缺口在默认演示质量与端到端编排，尤其是默认 duckduckgo 返回 0 结果时没有二级回退，也没有继续做抓取、重排、引用压缩。",
		"- 如果要把 web 搜索补到更接近别的框架的完整体验，下一步应优先补：1) provider 优先级回退链；2) search -> fetch/crawl -> rerank -> citation summary 主链；3) UI/报告里展示来源链接与命中域名；4) 为 Tavily / Firecrawl / SearXNG 增加可直接运行的示例。这里是基于仓内代码和官方文档的推断。",
	}, "\n")
}

func buildWebSearchBenchmarkHTML() string {
	items := []string{
		"当前仓内已具备两层 web 搜索能力：provider-native（OpenAI Responses、Anthropic Messages、Gemini grounding）与 hosted provider-backed（tavily / firecrawl / duckduckgo / searxng）。",
		"本次已把 <code>examples/07_mid_priority_features</code> 从假 endpoint 改成真实 provider-backed hosted <code>web_search</code>；无 API key 时默认走 duckduckgo。",
		`OpenAI 官方文档说明 Responses API 的 <code>web_search</code> 支持 citations、sources 列表、domain filtering、<code>external_web_access</code>，并在 reasoning 模型下支持 <code>open_page</code> / <code>find_in_page</code>。文档：<a href="https://platform.openai.com/docs/guides/tools-web-search">OpenAI Web search</a>`,
		`Anthropic 官方文档说明 <code>web_search_20250305</code> 支持 <code>max_uses</code>、<code>allowed_domains</code> / <code>blocked_domains</code>、approximate <code>user_location</code>，Claude 最终回答会带来源引用。文档：<a href="https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool">Anthropic Web search tool</a>`,
		`Google Vertex AI 官方文档说明 Gemini grounding 不仅支持 Google Search，还支持 Vertex AI Search 与 external search API，并允许组合多种 grounding source。文档：<a href="https://docs.cloud.google.com/vertex-ai/generative-ai/docs/grounding/overview">Grounding overview</a>`,
		`LangChain 官方文档里的常见路线是 Tavily / SerpAPI 等外部搜索工具，再叠加网页浏览或抓取工具；Google 集成还直接暴露 Gemini native tools。文档：<a href="https://docs.langchain.com/langsmith/test-react-agent-pytest">LangSmith ReAct + Tavily</a> / <a href="https://docs.langchain.com/oss/javascript/integrations/tools/google">LangChain Google tools</a> / <a href="https://docs.langchain.com/oss/javascript/integrations/tools/webbrowser/">LangChain WebBrowser</a>`,
		"结合本仓代码与这些官方能力做判断：AgentFlow 现在“接口面”已经不缺，主要缺口在默认演示质量与端到端编排，尤其是默认 duckduckgo 返回 0 结果时没有二级回退，也没有继续做抓取、重排、引用压缩。",
		"如果要把 web 搜索补到更接近别的框架的完整体验，下一步应优先补：provider 优先级回退链、search -> fetch/crawl -> rerank -> citation summary 主链、报告/UI 中的来源链接展示，以及 Tavily / Firecrawl / SearXNG 的可直接运行示例。这里是基于仓内代码和官方文档的推断。",
	}

	var b strings.Builder
	b.WriteString("<ul>")
	for _, item := range items {
		b.WriteString("<li>" + item + "</li>")
	}
	b.WriteString("</ul>")
	return b.String()
}

func hasLinePrefix(output, prefix string) bool {
	lines := nonEmptyLines(output)
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func repoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func errorAs(err error, target any) bool {
	switch t := target.(type) {
	case **exec.ExitError:
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			*t = exitErr
		}
		return ok
	default:
		return false
	}
}

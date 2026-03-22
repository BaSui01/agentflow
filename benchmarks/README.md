# Benchmarks

本目录存放 Go benchmark 基线文件和最新运行结果，用于性能回归检测。

## 快速开始

```bash
# 运行全部 benchmark 并保存到 benchmarks/latest.txt
./scripts/benchmark.sh

# 手动运行（不保存文件）
go test -bench=. -benchmem -count=3 -timeout 120s \
  ./llm/providers/openaicompat/ \
  ./llm/capabilities/tools/ \
  ./agent/memorycore/
```

## 对比基线

需要先安装 `benchstat`：

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

然后对比：

```bash
benchstat benchmarks/baseline_v1.7.7.txt benchmarks/latest.txt
```

输出中关注 `delta` 列：
- `~` 表示无显著变化
- `+XX%` 表示性能回退（需要关注）
- `-XX%` 表示性能提升

## 更新基线

当性能变化是预期的（如新功能引入合理开销），更新基线：

```bash
cp benchmarks/latest.txt benchmarks/baseline_v<NEW_VERSION>.txt
git add benchmarks/
git commit -m "bench: update baseline to v<NEW_VERSION>"
```

## CI 集成

CI 中的 `benchmark` job 会自动：
1. 运行 benchmark（`-count=3` 取多次采样）
2. 与最新的 `baseline_*.txt` 做 `benchstat` 对比
3. 将对比结果写入 GitHub Actions Summary
4. 上传 `benchmark-report` artifact（保留 14 天）

查看方式：进入 Actions 运行页面 → 点击 Summary 查看对比表，或下载 artifact 中的 `benchstat-diff.txt`。

## 文件说明

| 文件 | 用途 |
|------|------|
| `baseline_v*.txt` | 版本基线，用于 benchstat 对比 |
| `latest.txt` | 本地最近一次运行结果（gitignore） |

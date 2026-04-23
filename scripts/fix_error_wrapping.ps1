# 自动修复错误包装：将 %v 改为 %w（当参数是 error 类型时）
# 注意：这个脚本只处理明确是 error 类型的参数

$ErrorActionPreference = "Stop"

Write-Host "=== 开始修复错误包装问题 ===" -ForegroundColor Green

# 统计当前问题数量
$beforeCount = (Get-ChildItem -Path . -Include *.go -Recurse -File | Select-String -Pattern 'fmt\.Errorf.*%v' | Measure-Object).Count
Write-Host "修复前: 发现 $beforeCount 处使用 %v 的地方" -ForegroundColor Yellow

# 需要修复的文件列表（基于搜索结果）
$filesToFix = @(
    "agent/builder.go",
    "agent/discovery/composer.go",
    "agent/discovery/executor.go",
    "agent/protocol/a2a/client.go",
    "agent/streaming/bidirectional.go",
    "config/hotreload.go",
    "llm/circuitbreaker/breaker.go"
)

foreach ($file in $filesToFix) {
    if (Test-Path $file) {
        Write-Host "检查文件: $file" -ForegroundColor Cyan
        
        # 读取文件内容
        $content = Get-Content $file -Raw -Encoding UTF8
        $originalContent = $content
        
        # 修复模式1: fmt.Errorf("...%v", err) -> fmt.Errorf("...%w", err)
        # 只修复参数名包含 err, error, Error 的情况
        $content = $content -replace 'fmt\.Errorf\(("([^"]*%v[^"]*)",\s*(\w*[Ee]rr\w*))\)', 'fmt.Errorf("$2", $3)'
        
        # 修复模式2: fmt.Errorf("%w: %v", ErrXXX, err) -> fmt.Errorf("%w: %w", ErrXXX, err)
        $content = $content -replace 'fmt\.Errorf\("%w: %v",', 'fmt.Errorf("%w: %w",'
        
        # 修复模式3: fmt.Errorf("...%v", lastErr) -> fmt.Errorf("...%w", lastErr)
        $content = $content -replace 'fmt\.Errorf\(("([^"]*%v[^"]*)",\s*(lastErr))\)', 'fmt.Errorf("$2", $3)'
        
        if ($content -ne $originalContent) {
            Set-Content $file $content -Encoding UTF8 -NoNewline
            Write-Host "  ✓ 已修复: $file" -ForegroundColor Green
        } else {
            Write-Host "  - 无需修改: $file" -ForegroundColor Gray
        }
    }
}

# 统计修复后的数量
$afterCount = (Get-ChildItem -Path . -Include *.go -Recurse -File | Select-String -Pattern 'fmt\.Errorf.*%v' | Measure-Object).Count
Write-Host "`n修复后: 剩余 $afterCount 处使用 %v 的地方" -ForegroundColor Yellow
Write-Host "已修复: $($beforeCount - $afterCount) 处" -ForegroundColor Green

Write-Host "`n=== 修复完成 ===" -ForegroundColor Green
Write-Host "注意: 剩余的 %v 可能是合法用法（如 panic recover 的 any 类型）" -ForegroundColor Yellow

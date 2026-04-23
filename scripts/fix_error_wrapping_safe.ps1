# 安全修复错误包装 - 只处理明确的 error 类型参数

$ErrorActionPreference = "Stop"

Write-Host "=== 开始安全修复错误包装问题 ===" -ForegroundColor Green

# 定义需要修复的文件和具体的修复模式
$fixes = @(
    @{
        File = "agent/builder.go"
        Patterns = @(
            @{
                Old = 'fmt.Errorf("builder has %d errors: %v", len(b.errors), b.errors[0])'
                New = 'fmt.Errorf("builder has %d errors: %w", len(b.errors), b.errors[0])'
            }
        )
    },
    @{
        File = "agent/discovery/composer.go"
        Patterns = @(
            @{
                Old = 'fmt.Errorf("incomplete composition: missing capabilities %v", missingCapabilities)'
                New = 'fmt.Errorf("incomplete composition: missing capabilities: %v", missingCapabilities)'
            }
        )
    },
    @{
        File = "agent/discovery/executor.go"
        Patterns = @(
            @{
                Old = 'fmt.Errorf("composition is incomplete: missing capabilities %v", result.MissingCapabilities)'
                New = 'fmt.Errorf("composition is incomplete: missing capabilities: %v", result.MissingCapabilities)'
            }
        )
    }
)

$fixedCount = 0

foreach ($fix in $fixes) {
    $file = $fix.File
    if (-not (Test-Path $file)) {
        Write-Host "文件不存在: $file" -ForegroundColor Yellow
        continue
    }

    $content = Get-Content $file -Raw -Encoding UTF8
    $originalContent = $content

    foreach ($pattern in $fix.Patterns) {
        if ($content -match [regex]::Escape($pattern.Old)) {
            $content = $content -replace [regex]::Escape($pattern.Old), $pattern.New
            Write-Host "  ✓ 修复: $($pattern.Old.Substring(0, [Math]::Min(50, $pattern.Old.Length)))..." -ForegroundColor Cyan
        }
    }

    if ($content -ne $originalContent) {
        Set-Content $file $content -Encoding UTF8 -NoNewline
        Write-Host "✓ 已修复文件: $file" -ForegroundColor Green
        $fixedCount++
    }
}

Write-Host "`n=== 修复完成 ===" -ForegroundColor Green
Write-Host "已修复 $fixedCount 个文件" -ForegroundColor Yellow

# 验证编译
Write-Host "`n验证编译..." -ForegroundColor Cyan
go build ./... 2>&1 | Out-Null
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ 编译成功" -ForegroundColor Green
} else {
    Write-Host "✗ 编译失败，请检查错误" -ForegroundColor Red
}

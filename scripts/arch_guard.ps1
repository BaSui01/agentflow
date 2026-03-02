Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$strictMode = $env:ARCH_GUARD_STRICT -eq "1"
$maxFiles = 20
if ($env:ARCH_GUARD_MAX_FILES) {
    $maxFiles = [int]$env:ARCH_GUARD_MAX_FILES
}

function Get-GoProductionFiles {
    Get-ChildItem -Recurse -File -Filter "*.go" |
        Where-Object {
            $_.FullName -notmatch "_test\.go$" -and
            $_.FullName -notmatch "\\examples\\"
        }
}

$errors = @()
$warnings = @()

$files = Get-GoProductionFiles

# Rule 1: dependency direction guard
$pkgFiles = $files | Where-Object { $_.FullName -match "\\pkg\\" }
foreach ($file in $pkgFiles) {
    $matches = Select-String -Path $file.FullName -Pattern '"github.com/BaSui01/agentflow/api/' -SimpleMatch
    foreach ($m in $matches) {
        $rel = $file.FullName.Replace($root + "\", "").Replace("\", "/")
        $errors += "[LAYER] ${rel}:$($m.LineNumber) pkg layer must not import api layer"
    }
}

# Rule 2: workflow must not import agent/persistence
$workflowFiles = $files | Where-Object { $_.FullName -match "\\workflow\\" }
foreach ($file in $workflowFiles) {
    $matches = Select-String -Path $file.FullName -Pattern '"github.com/BaSui01/agentflow/agent/persistence' -SimpleMatch
    foreach ($m in $matches) {
        $rel = $file.FullName.Replace($root + "\", "").Replace("\", "/")
        $errors += "[LAYER] ${rel}:$($m.LineNumber) workflow must not import agent/persistence"
    }
}

# Rule 3: rag must not import agent/workflow/api/cmd
$ragFiles = $files | Where-Object { $_.FullName -match "\\rag\\" }
foreach ($file in $ragFiles) {
    $patterns = @(
        '"github.com/BaSui01/agentflow/agent/',
        '"github.com/BaSui01/agentflow/workflow/',
        '"github.com/BaSui01/agentflow/api/',
        '"github.com/BaSui01/agentflow/cmd/'
    )
    foreach ($pat in $patterns) {
        $matches = Select-String -Path $file.FullName -Pattern $pat -SimpleMatch
        foreach ($m in $matches) {
            $rel = $file.FullName.Replace($root + "\", "").Replace("\", "/")
            $errors += "[LAYER] ${rel}:$($m.LineNumber) rag layer must not import $pat"
        }
    }
}

# Rule 4: fat package guard
$groups = $files | Group-Object DirectoryName
foreach ($g in $groups) {
    $relDir = $g.Name.Replace($root + "\", "").Replace("\", "/")
    if ($g.Count -gt $maxFiles) {
        $warnings += "[SIZE] $relDir has $($g.Count) production files (threshold=$maxFiles)"
    }
}

# Rule 5: single-file package allowlist guard
$allowOneFile = @(
    ".",
    "internal/app/bootstrap",
    "internal/bridge",
    "pkg/openapi"
)

$oneFileDirs = $groups | Where-Object { $_.Count -eq 1 }
foreach ($g in $oneFileDirs) {
    $relDir = $g.Name.Replace($root + "\", "").Replace("\", "/")
    if ($allowOneFile -notcontains $relDir) {
        $warnings += "[SPLIT] $relDir is a single-file production package"
    }
}

if ($warnings.Count -gt 0) {
    Write-Host "Architecture warnings:" -ForegroundColor Yellow
    $warnings | Sort-Object | ForEach-Object { Write-Host "  $_" -ForegroundColor Yellow }
}

if ($strictMode -and $warnings.Count -gt 0) {
    $errors += $warnings
}

if ($errors.Count -gt 0) {
    Write-Host "Architecture errors:" -ForegroundColor Red
    $errors | Sort-Object | ForEach-Object { Write-Host "  $_" -ForegroundColor Red }
    exit 1
}

Write-Host "Architecture guard passed." -ForegroundColor Green

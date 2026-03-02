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

# Rule 0: dependency direction guard via go-arch-lint
$goArchLint = Get-Command go-arch-lint -ErrorAction SilentlyContinue
if ($null -eq $goArchLint) {
    $msg = "[DEPS] go-arch-lint not found in PATH; skip dependency direction checks"
    if ($strictMode) {
        $errors += $msg
    } else {
        $warnings += $msg
    }
} else {
    Write-Host "Running go-arch-lint check..." -ForegroundColor Cyan
    & $goArchLint.Source check
    if ($LASTEXITCODE -ne 0) {
        $msg = "[DEPS] go-arch-lint check failed"
        if ($strictMode) {
            $errors += $msg
        } else {
            $warnings += $msg
        }
    }
}

# Rule 1: fat package guard (not supported by go-arch-lint)
$groups = $files | Group-Object DirectoryName
foreach ($g in $groups) {
    $relDir = $g.Name.Replace($root + "\", "").Replace("\", "/")
    if ($g.Count -gt $maxFiles) {
        $warnings += "[SIZE] $relDir has $($g.Count) production files (threshold=$maxFiles)"
    }
}

# Rule 2: single-file package allowlist guard (not supported by go-arch-lint)
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

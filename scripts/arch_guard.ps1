Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$strictMode = $true
if ($env:ARCH_GUARD_STRICT -eq "0") {
    $strictMode = $false
}
$maxAgentRootFiles = 26
if ($env:ARCH_GUARD_MAX_FILES) {
    $maxAgentRootFiles = [int]$env:ARCH_GUARD_MAX_FILES
}

function Get-GoProductionFiles {
    Get-ChildItem -Recurse -File -Filter "*.go" |
        Where-Object {
            $_.FullName -notmatch "_test\.go$" -and
            $_.FullName -notmatch "\\examples\\" -and
            $_.FullName -notmatch "\\.snow\\" -and
            $_.FullName -notmatch "\\vendor\\"
        }
}

$errors = @()
$warnings = @()

$files = Get-GoProductionFiles

# Rule 0: dependency direction guard via go-arch-lint
$goArchLint = Get-Command go-arch-lint -ErrorAction SilentlyContinue
if ($null -eq $goArchLint) {
    $errors += "[DEPS] go-arch-lint not found in PATH"
} else {
    Write-Host "Running go-arch-lint check..." -ForegroundColor Cyan
    & $goArchLint.Source check
    if ($LASTEXITCODE -ne 0) {
        $errors += "[DEPS] go-arch-lint check failed"
    }
}

# Rule 1: root agent package file budget (aligned with architecture_guard_test.go)
$agentRootFiles = Get-ChildItem agent -File -Filter "*.go" |
    Where-Object { $_.Name -notmatch "_test\.go$" }
if ($agentRootFiles.Count -gt $maxAgentRootFiles) {
    $warnings += "[SIZE] agent root package has $($agentRootFiles.Count) production files (threshold=$maxAgentRootFiles)"
}

# Rule 2: single-file pkg directory allowlist (aligned with architecture_guard_test.go)
$allowOneFilePkg = @(
    "cache",
    "database",
    "jsonschema",
    "metrics",
    "openapi",
    "server",
    "telemetry",
    "tlsutil"
)

$pkgDirs = Get-ChildItem pkg -Directory
$actualOneFilePkg = @()
foreach ($dir in $pkgDirs) {
    $prodFiles = @(Get-ChildItem $dir.FullName -File -Filter "*.go" |
        Where-Object { $_.Name -notmatch "_test\.go$" })
    if ($prodFiles.Count -eq 1) {
        $actualOneFilePkg += $dir.Name
        if ($allowOneFilePkg -notcontains $dir.Name) {
            $warnings += "[SPLIT] pkg/$($dir.Name) is a new one-file package without architecture review"
        }
    }
}

foreach ($name in $allowOneFilePkg) {
    if ($actualOneFilePkg -notcontains $name) {
        $warnings += "[SPLIT] allowlist entry pkg/$name is stale, update architecture guard"
    }
}

# Rule 3: config-driven chat provider entry must stay on vendor factory path.
$vendorEntryFiles = @(
    "agentflow.go",
    "llm/runtime/compose/main_provider_registry.go",
    "llm/runtime/compose/runtime.go",
    "llm/runtime/router/chat_provider_factory.go"
)
foreach ($path in $vendorEntryFiles) {
    $content = Get-Content -Path $path -Raw
    switch ($path) {
        "agentflow.go" {
            if ($content -notmatch "vendor\.NewChatProviderFromConfig\(") {
                $errors += "[LLM] $path must use vendor.NewChatProviderFromConfig(...)"
            }
        }
        "llm/runtime/router/chat_provider_factory.go" {
            if ($content -notmatch "vendor\.NewChatProviderFromConfig\(") {
                $errors += "[LLM] $path must route provider construction through vendor.NewChatProviderFromConfig(...)"
            }
        }
        default {
            if ($content -notmatch "VendorChatProviderFactory") {
                $errors += "[LLM] $path must keep VendorChatProviderFactory as the config-driven provider entry"
            }
            if ($content -match "NewOpenAIProvider\(" -or $content -match "NewClaudeProvider\(" -or $content -match "NewGeminiProvider\(") {
                $errors += "[LLM] $path must not hardcode direct provider constructors"
            }
        }
    }
}

# Rule 4: public multi-provider docs must demonstrate the vendor factory path.
$providerRoutingDocs = @(
    "README.md",
    "README_EN.md",
    "docs/cn/tutorials/02.Provider配置指南.md",
    "docs/en/tutorials/02.ProviderConfiguration.md"
)
foreach ($path in $providerRoutingDocs) {
    $content = Get-Content -Path $path -Raw
    if ($content -notmatch "VendorChatProviderFactory") {
        $errors += "[DOCS] $path must document VendorChatProviderFactory for multi-provider routing"
    }
    if ($content -match "NewDefaultProviderFactory\(") {
        $errors += "[DOCS] $path still documents legacy NewDefaultProviderFactory() for public multi-provider routing"
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

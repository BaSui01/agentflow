Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$strictMode = $true
if ($env:ARCH_GUARD_STRICT -eq "0") {
    $strictMode = $false
}
$maxAgentRootFiles = 0
if ($env:ARCH_GUARD_MAX_FILES) {
    $maxAgentRootFiles = [int]$env:ARCH_GUARD_MAX_FILES
}

# Stabilize local Go builds on this machine to reduce cache corruption / OOM noise.
# Allow explicit caller overrides when needed.
$originalGoCache = $env:GOCACHE
$originalGoFlags = $env:GOFLAGS
$originalGoMaxProcs = $env:GOMAXPROCS

if (-not $env:ARCH_GUARD_GOCACHE) {
    $env:ARCH_GUARD_GOCACHE = (Join-Path $root ".tmp/gocache")
}
if (-not $env:GOCACHE) {
    $env:GOCACHE = $env:ARCH_GUARD_GOCACHE
}
New-Item -ItemType Directory -Force -Path $env:GOCACHE | Out-Null

if (-not $env:ARCH_GUARD_GOFLAGS) {
    $env:ARCH_GUARD_GOFLAGS = "-p=1"
}
if (-not $env:GOFLAGS) {
    $env:GOFLAGS = $env:ARCH_GUARD_GOFLAGS
}

if (-not $env:ARCH_GUARD_GOMAXPROCS) {
    $env:ARCH_GUARD_GOMAXPROCS = "1"
}
if (-not $env:GOMAXPROCS) {
    $env:GOMAXPROCS = $env:ARCH_GUARD_GOMAXPROCS
}

Write-Host "Architecture guard Go env: GOCACHE=$($env:GOCACHE) GOFLAGS=$($env:GOFLAGS) GOMAXPROCS=$($env:GOMAXPROCS)" -ForegroundColor DarkCyan

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
$agentRootFiles = @(Get-ChildItem agent -File -Filter "*.go" |
    Where-Object { $_.Name -notmatch "_test\.go$" })
if ($agentRootFiles.Count -ne $maxAgentRootFiles) {
    $errors += "[SIZE] agent root package must have $maxAgentRootFiles production files; found $($agentRootFiles.Count)"
}

# Rule 1.1: rag/workflow/llm roots also must stay zero-file after destructive migration.
$zeroRootModules = @("rag", "workflow", "llm")
foreach ($module in $zeroRootModules) {
    $moduleRootFiles = @(Get-ChildItem $module -File -Filter "*.go" |
        Where-Object { $_.Name -notmatch "_test\.go$" })
    if ($moduleRootFiles.Count -ne 0) {
        $errors += "[SIZE] $module root package must have 0 production files; found $($moduleRootFiles.Count)"
    }
}

# Rule 2: single-file pkg directory allowlist (aligned with architecture_guard_test.go)
$allowOneFilePkg = @(
    "cache",
    "database",
    "jsonschema",
    "metrics",
    "openapi",
    "server",
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
    "internal/app/bootstrap/main_provider_registry.go",
    "llm/runtime/compose/runtime.go",
    "llm/runtime/router/chat_provider_factory.go"
)
foreach ($path in $vendorEntryFiles) {
    $content = Get-Content -Path $path -Raw
    switch ($path) {
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

# Rule 3.5: llm/runtime/compose must stay free of startup config / gorm coupling.
$composeFiles = Get-ChildItem "llm/runtime/compose" -File -Filter "*.go" |
    Where-Object { $_.Name -notmatch "_test\.go$" }
foreach ($file in $composeFiles) {
    $content = Get-Content -Path $file.FullName -Raw
    $rel = Resolve-Path -Relative $file.FullName
    if ($content -match 'github.com/BaSui01/agentflow/config' -or $content -match 'gorm.io/gorm') {
        $errors += "[LLM] $rel must not import startup config or gorm; move main-provider assembly to bootstrap"
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

# Rule 4.25: public README/docs must not promote legacy agent/workflow entrypoints.
$legacyUnifiedEntryDocs = @(
    "README.md",
    "README_EN.md",
    "docs/cn/getting-started/02.框架入口与快速开始.md",
    "docs/cn/tutorials/01.快速开始.md",
    "docs/en/tutorials/01.QuickStart.md",
    "docs/cn/tutorials/03.Agent开发教程.md",
    "docs/en/tutorials/03.AgentDevelopment.md",
    "docs/cn/tutorials/05.工作流编排.md",
    "docs/cn/guides/best-practices.md"
)
$legacyUnifiedEntryPatterns = @(
    "agent\.NewAgentBuilder\(",
    "agent\.BuildBaseAgent\(",
    "agent\.CreateAgent\(",
    "DAGWorkflow\.Execute\("
)
foreach ($path in $legacyUnifiedEntryDocs) {
    $content = Get-Content -Path $path -Raw
    foreach ($pattern in $legacyUnifiedEntryPatterns) {
        if ($content -match $pattern) {
            $errors += "[DOCS] $path must not promote legacy public entry pattern '$pattern'; use the unified runtime/facade entry instead"
        }
    }
}

# Rule 4.5: protected business-layer packages must not direct-call provider Completion/Stream.
$protectedGatewayDirs = @(
    "workflow",
    "agent/reasoning",
    "agent/structured",
    "agent/evaluation",
    "agent/deliberation"
)
foreach ($dir in $protectedGatewayDirs) {
    if (-not (Test-Path $dir)) {
        continue
    }
    $matches = rg -n '\.(Completion|Stream)\(' $dir -g '!**/*_test.go'
    if ($LASTEXITCODE -eq 0 -and $matches) {
        $matches -split "`n" | Where-Object { $_.Trim() -ne "" } | ForEach-Object {
            $errors += "[GATEWAY] direct provider call forbidden: $_"
        }
    } elseif ($LASTEXITCODE -gt 1) {
        $errors += "[GATEWAY] failed to scan $dir for direct provider calls"
    }
}

# Rule 5.5: agent/ 顶层目录必须严格等于 8 层 allowlist；Phase-5 清零的目录不得回潮。
$agentAllowedTopDirs = @(
    "adapters",
    "capabilities",
    "collaboration",
    "core",
    "execution",
    "integration",
    "observability",
    "persistence",
    "runtime",
    "team"
)
$agentBannedTopDirs = @(
    # Phase-2~4 合并/下沉的目录
    "memorycore",
    "guardcore",
    "deliberation",
    "teamadapter",
    "crews",
    "discovery",
    "longrunning",
    # Phase-5 清零的 20 个空目录
    "artifacts",
    "context",
    "conversation",
    "declarative",
    "deployment",
    "evaluation",
    "handoff",
    "hitl",
    "hosted",
    "k8s",
    "lsp",
    "multiagent",
    "orchestration",
    "planner",
    "reasoning",
    "skills",
    "streaming",
    "structured",
    "voice",
    # Phase-5 合并到 agent/core/
    "internalcore"
)
$agentTopDirs = Get-ChildItem agent -Directory | Select-Object -ExpandProperty Name
foreach ($dir in $agentTopDirs) {
    if ($agentBannedTopDirs -contains $dir) {
        $errors += "[LAYOUT] agent/$dir/ 已在 Phase-2~5 清零，禁止回潮；应保持 8 层 allowlist"
        continue
    }
    if ($agentAllowedTopDirs -notcontains $dir) {
        $errors += "[LAYOUT] agent/$dir/ 不在 8 层 allowlist (adapters/capabilities/collaboration/core/execution/integration/observability/persistence/runtime/team)"
    }
}
foreach ($required in $agentAllowedTopDirs) {
    if ($agentTopDirs -notcontains $required) {
        $errors += "[LAYOUT] 目标架构层 agent/$required/ 缺失，重构未完成"
    }
}

# Rule 5: architecture guard tests must pass, including README layer map / matrix checks.
Write-Host "Running focused architecture guard tests..." -ForegroundColor Cyan
& go test -run "Test(ReadmeCmdAgentflowStructureConsistency|ReadmeLayerMapAndMatrixConsistency|DependencyDirectionGuards|LLMComposeImportGuards|APIHandlerStoreLeakGuards|APIHandlerInfraImportGuards|CmdEntrypointImportAllowlist|CmdHotReloadBootstrapEntryPoints|HotReloadDocsImplementationConsistency|UsecaseContractBoundaryGuards|GatewayDirectProviderCallGuards|AgentUnifiedBuilderEntryPoints|PublicUnifiedEntrypointDocs|AgentOfficialRuntimeEntrypointDocs|OfficialEntrypointDocsConsistency|PublicProductSurfaceDocsExamplesConsistency|AgentExecutionOptionsArchitectureGuards|UsecaseUsesOfficialTeamExecutionFacade|NonTeamProductionCodeAvoidsModeRegistryDefaults|TutorialsDoNotRecommendTeamInternalPackages|ExamplesDoNotImportTeamInternalPackages|TeamPublicSurfaceDoesNotReexportInternalEngines|ExamplesAndLivecheckUseOfficialTeamFacade|AgentRootPackageFileBudget|AgentRootPublicSurfaceBudget|RAGRootPackageFileBudget|WorkflowRootPackageFileBudget|LLMRootPackageFileBudget|RootLayoutBudget|PkgOneFileDirectoryAllowlist)$" .
if ($LASTEXITCODE -ne 0) {
    $errors += "[TEST] focused architecture guard tests failed"
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
    $env:GOCACHE = $originalGoCache
    $env:GOFLAGS = $originalGoFlags
    $env:GOMAXPROCS = $originalGoMaxProcs
    exit 1
}

$env:GOCACHE = $originalGoCache
$env:GOFLAGS = $originalGoFlags
$env:GOMAXPROCS = $originalGoMaxProcs

Write-Host "Architecture guard passed." -ForegroundColor Green

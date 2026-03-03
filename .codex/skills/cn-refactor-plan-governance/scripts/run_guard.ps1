param(
  [ValidateSet("lint", "report", "gate")]
  [string]$Cmd = "lint",
  [string]$Target = "all"
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "../../../..")
Set-Location $repoRoot

python scripts/refactor_plan_guard.py $Cmd --target $Target
exit $LASTEXITCODE


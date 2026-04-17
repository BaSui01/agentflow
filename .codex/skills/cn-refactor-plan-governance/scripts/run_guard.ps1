param(
  [ValidateSet("lint", "report", "gate")]
  [string]$Cmd = "lint",
  [string]$Target = "all",
  [switch]$RequireTDD,
  [switch]$RequireVerifiableCompletion
)

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "../../../..")
Set-Location $repoRoot

$args = @($Cmd, "--target", $Target)
if ($RequireTDD) {
  $args += "--require-tdd"
}
if ($RequireVerifiableCompletion) {
  $args += "--require-verifiable-completion"
}

python scripts/refactor_plan_guard.py @args
exit $LASTEXITCODE


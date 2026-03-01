param(
    [int]$BatchSize = 10,
    [string]$BranchName = ""
)

$t = Get-Date -Format "yyyyMMdd-HHmm"
if ([string]::IsNullOrWhiteSpace($BranchName) -or $BranchName -eq "batch/auto (推荐)") {
    $BranchName = "batch/auto-$t"
}
Write-Output "Using branch: $BranchName"

# Create and switch to new branch
git switch -c "$BranchName"
if ($LASTEXITCODE -ne 0) { Write-Error "Failed to create/switch branch $BranchName"; exit 2 }

# Get porcelain status
$porcelain = git status --porcelain
if (-not $porcelain) {
    Write-Output "No changes to commit."
    exit 0
}

# Parse changed file paths
$files = @()
$porcelain -split "`n" | ForEach-Object {
    if ($_.Length -gt 3) {
        $files += $_.Substring(3).Trim()
    }
}
$files = $files | Where-Object { $_ -ne "" }

$total = $files.Count
Write-Output "Found $total changed files"
if ($total -eq 0) { Write-Output "No files parsed to commit."; exit 0 }

$batch = 1
for ($i = 0; $i -lt $total; $i += $BatchSize) {
    $end = [math]::Min($i + $BatchSize - 1, $total - 1)
    $subset = $files[$i..$end]
    Write-Output "Staging batch $batch (files $($i+1)-$($end+1)): $($subset -join ', ')"
    git add -- $subset
    if ($LASTEXITCODE -ne 0) { Write-Error "git add failed for batch $batch"; exit 3 }
    $startIndex = $i + 1
    $endIndex = $end + 1
    git commit -m "chore(batch $batch): commit files $startIndex-$endIndex"
    if ($LASTEXITCODE -ne 0) { Write-Error "git commit failed for batch $batch"; exit 4 }
    $batch++
}

# Push branch
git push -u origin "$BranchName"
if ($LASTEXITCODE -ne 0) { Write-Error "git push failed"; exit 5 }

Write-Output "Done. Created branch '$BranchName', committed in $($batch-1) batches, and pushed to origin."

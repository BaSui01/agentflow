param(
    [string]$Source = "llm/providers/capability_matrix.go",
    [string]$Out = "docs/generated/llm-implemented-matrix.md"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

if (-not (Test-Path $Source)) {
    throw "source file not found: $Source"
}

$content = Get-Content -Path $Source -Raw

# Parse entries like:
# {Provider: "OpenAI", Image: true, Video: false, AudioGenerate: true, AudioSTT: true, Embedding: true, FineTuning: true},
$entryPattern = '\{Provider:\s*"(?<Provider>[^"]+)",\s*Image:\s*(?<Image>true|false),\s*Video:\s*(?<Video>true|false),\s*AudioGenerate:\s*(?<AudioGenerate>true|false),\s*AudioSTT:\s*(?<AudioSTT>true|false),\s*Embedding:\s*(?<Embedding>true|false),\s*FineTuning:\s*(?<FineTuning>true|false)\}'
$matches = [regex]::Matches($content, $entryPattern)

if ($matches.Count -eq 0) {
    throw "no capability entries parsed from: $Source"
}

function Mark([string]$v) {
    if ($v -eq "true") { return "✅" }
    return "❌"
}

$lines = New-Object System.Collections.Generic.List[string]
$lines.Add("| Provider | 图像 | 视频 | 音频生成 | 音频转录 | Embedding | 微调 |")
$lines.Add("|---|---|---|---|---|---|---|")

foreach ($m in $matches) {
    $provider = $m.Groups["Provider"].Value
    $image = Mark $m.Groups["Image"].Value
    $video = Mark $m.Groups["Video"].Value
    $audioGenerate = Mark $m.Groups["AudioGenerate"].Value
    $audioSTT = Mark $m.Groups["AudioSTT"].Value
    $embedding = Mark $m.Groups["Embedding"].Value
    $fineTuning = Mark $m.Groups["FineTuning"].Value
    $lines.Add("| $provider | $image | $video | $audioGenerate | $audioSTT | $embedding | $fineTuning |")
}

$outDir = Split-Path -Parent $Out
if (-not [string]::IsNullOrWhiteSpace($outDir)) {
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null
}

$lines -join "`n" | Set-Content -Path $Out -Encoding UTF8
Write-Output "generated $Out"

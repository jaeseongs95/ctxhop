param(
    [Parameter(Mandatory = $true)]
    [string]$GoExe,
    [string]$LogDir = (Join-Path $PSScriptRoot 'verification-results')
)

$ErrorActionPreference = 'Stop'
$taskGoExe = (Resolve-Path -LiteralPath $GoExe).Path
$taskWorkRoot = 'D:\Go\temp\ctxhop-go-regression-work'
$taskEnvironment = @{
    TMP = 'D:\Go\temp'
    TEMP = 'D:\Go\temp'
    GOTMPDIR = 'D:\Go\temp'
    GOTOOLCHAIN = 'local'
    GOPATH = (Join-Path $taskWorkRoot 'gopath')
    GOCACHE = (Join-Path $taskWorkRoot 'gocache')
}
$taskPrevious = @{}
foreach ($taskName in $taskEnvironment.Keys) {
    $taskPrevious[$taskName] = [Environment]::GetEnvironmentVariable($taskName, 'Process')
}
Push-Location $PSScriptRoot
try {
    New-Item -ItemType Directory -Path 'D:\Go\temp' -Force | Out-Null
    if (-not (Test-Path -LiteralPath 'D:\Go\temp' -PathType Container)) {
        throw 'AGENTS.md requires the available test root D:\Go\temp.'
    }
    New-Item -ItemType Directory -Path $LogDir -Force | Out-Null
    foreach ($taskName in $taskEnvironment.Keys) {
        [Environment]::SetEnvironmentVariable($taskName, $taskEnvironment[$taskName], 'Process')
    }
    & $taskGoExe test ./cmd/ctxhop -run 'TestResumeNoEnvironmentFlagAndHelp|TestSessionResumeNoEnvironmentAfterRemoteChange' -count=1 -v 2>&1 |
        Tee-Object -FilePath (Join-Path $LogDir 'targeted-regression.txt')
    if ($LASTEXITCODE -ne 0) { throw 'Targeted regression failed.' }
    & $taskGoExe test ./cmd/ctxhop ./internal/environment ./internal/adapter ./internal/syncflow ./internal/syncer -count=1 2>&1 |
        Tee-Object -FilePath (Join-Path $LogDir 'relevant-suites.txt')
    if ($LASTEXITCODE -ne 0) { throw 'Relevant upstream suites failed.' }
    Set-Content -LiteralPath (Join-Path $LogDir 'vet.txt') -Value '' -NoNewline
    & $taskGoExe vet ./cmd/ctxhop 2>&1 |
        Tee-Object -FilePath (Join-Path $LogDir 'vet.txt')
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed.' }
}
finally {
    foreach ($taskName in $taskPrevious.Keys) {
        [Environment]::SetEnvironmentVariable($taskName, $taskPrevious[$taskName], 'Process')
    }
    Pop-Location
}

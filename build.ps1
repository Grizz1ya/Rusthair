$ErrorActionPreference = 'Stop'
$go = Get-Command go.exe -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source
if (-not $go) {
    $go = Join-Path $env:LOCALAPPDATA 'RusthairBuild\go\bin\go.exe'
    if (-not (Test-Path -LiteralPath $go)) { throw 'Install Go 1.23 or later to build Rusthair.' }
}
Push-Location $PSScriptRoot
try {
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    & $go build -trimpath -buildvcs=false -ldflags '-H=windowsgui' -o Rusthair.exe .
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed.' }
    Get-Item -LiteralPath 'Rusthair.exe' | Select-Object FullName, Length
} finally {
    Pop-Location
}

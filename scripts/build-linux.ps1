param(
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)
New-Item -ItemType Directory -Force dist | Out-Null

$env:CGO_ENABLED = "0"
foreach ($arch in @("amd64", "arm64")) {
    $env:GOOS = "linux"
    $env:GOARCH = $arch
    $output = "dist/heimdrive-linux-$arch"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $output .
    $size = [math]::Round((Get-Item $output).Length / 1MB, 1)
    Write-Host "Gebaut: $output ($size MB)"
}

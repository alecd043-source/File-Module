param(
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $PSScriptRoot)
New-Item -ItemType Directory -Force dist | Out-Null

$env:CGO_ENABLED = "0"
foreach ($arch in @("amd64", "arm64", "386")) {
    $env:GOOS = "linux"
    $env:GOARCH = $arch
    if ($arch -eq "386") {
        # Also works on old 32-bit CPUs without SSE2 (e.g. Athlon XP).
        $env:GO386 = "softfloat"
    } else {
        Remove-Item Env:GO386 -ErrorAction SilentlyContinue
    }
    $output = "dist/heimdrive-linux-$arch"
    go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $output .
    $size = [math]::Round((Get-Item $output).Length / 1MB, 1)
    Write-Host "Gebaut: $output ($size MB)"
}

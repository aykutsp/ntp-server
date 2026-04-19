param(
  [string]$Version = "dev",
  [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
$pkg = "./cmd/ntp-server"

if (-not (Test-Path $OutDir)) {
  New-Item -ItemType Directory -Path $OutDir | Out-Null
}

$targets = @(
  @{ os = "linux"; arch = "amd64" },
  @{ os = "linux"; arch = "arm64" },
  @{ os = "darwin"; arch = "amd64" },
  @{ os = "darwin"; arch = "arm64" },
  @{ os = "windows"; arch = "amd64" }
)

foreach ($target in $targets) {
  $name = "ntp-server-$($target.os)-$($target.arch)"
  if ($target.os -eq "windows") {
    $name = "$name.exe"
  }

  Write-Host "Building $name"
  $env:CGO_ENABLED = "0"
  $env:GOOS = $target.os
  $env:GOARCH = $target.arch
  go build -trimpath -ldflags "-s -w -X main.version=$Version" -o "$OutDir/$name" $pkg
}

Write-Host "Build artifacts are in $OutDir/"

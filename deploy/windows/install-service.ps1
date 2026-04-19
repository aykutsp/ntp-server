param(
  [string]$ServiceName = "NtpServer",
  [string]$DisplayName = "NTP Server",
  [string]$BinaryPath = "C:\Program Files\NtpServer\ntp-server.exe",
  [string]$ConfigPath = "C:\ProgramData\NtpServer\config.json"
)

$ErrorActionPreference = "Stop"

if (-not ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")) {
  throw "This script must be run as Administrator."
}

if (-not (Test-Path $BinaryPath)) {
  throw "Binary not found: $BinaryPath"
}
if (-not (Test-Path $ConfigPath)) {
  throw "Config not found: $ConfigPath"
}

$binWithArgs = "`"$BinaryPath`" -config `"$ConfigPath`""

if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
  Write-Host "Service already exists. Updating configuration..."
  sc.exe config $ServiceName binPath= $binWithArgs start= auto | Out-Null
} else {
  sc.exe create $ServiceName binPath= $binWithArgs start= auto DisplayName= $DisplayName | Out-Null
}

sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null
sc.exe description $ServiceName "Reliable NTP service." | Out-Null

Start-Service -Name $ServiceName
Write-Host "Service '$ServiceName' installed and started."

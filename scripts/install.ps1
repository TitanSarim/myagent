# Install myagent on Windows (PowerShell)
# Usage:
#   irm https://raw.githubusercontent.com/TitanSarim/myagent/main/scripts/install.ps1 | iex
# Or download/run this file locally.

$ErrorActionPreference = "Stop"
$Version = if ($env:MYAGENT_VERSION) { $env:MYAGENT_VERSION } else { "0.4.0" }
$Repo = "TitanSarim/myagent"
$Arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "amd64" }
$Asset = "myagent_${Version}_windows_${Arch}.exe"
$Url = "https://github.com/$Repo/releases/download/v$Version/$Asset"

$BinDir = Join-Path $env:LOCALAPPDATA "myagent"
$Dest = Join-Path $BinDir "myagent.exe"

New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
Write-Host "Downloading $Url"
Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing

# Add to user PATH if missing
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$BinDir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$BinDir", "User")
  $env:Path = "$env:Path;$BinDir"
  Write-Host "Added to user PATH: $BinDir"
}

# Default config pointing at Linux Ollama on the LAN (edit if needed)
$cfgDir = Join-Path $env:APPDATA "localcode"
New-Item -ItemType Directory -Force -Path $cfgDir | Out-Null
$cfg = Join-Path $cfgDir "config.yaml"
if (-not (Test-Path $cfg)) {
  @"
provider:
  type: ollama
  base_url: http://192.168.8.22:11434
models:
  fast:
    name: qwen3.5:9b
    context: 16384
    keep_alive: 2m
  smart:
    name: qwen3.8:27b
    context: 8192
    keep_alive: 2m
  code:
    name: qwen3.6:27b
    context: 8192
    keep_alive: 2m
routing:
  default: auto
  one_model_per_request: true
safety:
  require_patch_approval: true
  require_dangerous_command_approval: true
  restrict_to_repo: true
"@ | Set-Content -Path $cfg -Encoding UTF8
  Write-Host "Wrote config: $cfg"
}

Write-Host ""
Write-Host "Installed: $Dest"
Write-Host "Open a NEW Command Prompt / PowerShell, then run:"
Write-Host "  myagent"
Write-Host "Or without PATH refresh:"
Write-Host "  & `"$Dest`""

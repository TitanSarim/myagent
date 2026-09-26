# Run in PowerShell as Administrator on the Windows PC (192.168.8.21)
# Makes Ollama reachable on the LAN so myagent on another PC can call it.

$ErrorActionPreference = "Stop"

Write-Host "==> Configuring Ollama for LAN access..." -ForegroundColor Cyan

# 1) Listen on all interfaces (not just localhost)
[System.Environment]::SetEnvironmentVariable("OLLAMA_HOST", "0.0.0.0:11434", "Machine")
$env:OLLAMA_HOST = "0.0.0.0:11434"

# 2) Allow port 11434 through Windows Firewall
$ruleName = "Ollama LAN 11434"
Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue | Remove-NetFirewallRule
New-NetFirewallRule -DisplayName $ruleName -Direction Inbound -Protocol TCP -LocalPort 11434 -Action Allow | Out-Null
Write-Host "Firewall rule added: $ruleName"

# 3) Restart Ollama app / service if present
Get-Process ollama -ErrorAction SilentlyContinue | Stop-Process -Force
$ollama = "$env:LOCALAPPDATA\Programs\Ollama\ollama.exe"
if (Test-Path $ollama) {
  Start-Process $ollama -ArgumentList "serve"
  Write-Host "Started: $ollama serve"
} else {
  Write-Host "Start Ollama from the Start Menu (Quit + reopen the tray app)." -ForegroundColor Yellow
}

Start-Sleep -Seconds 2

# 4) Local smoke test
try {
  $tags = Invoke-RestMethod -Uri "http://127.0.0.1:11434/api/tags" -TimeoutSec 5
  Write-Host "Local Ollama OK — models:" ($tags.models | ForEach-Object { $_.name }) -join ", "
} catch {
  Write-Host "Local check failed. Is Ollama installed and running?" -ForegroundColor Red
  throw
}

Write-Host ""
Write-Host "Done. From your Linux PC (192.168.8.22) test with:" -ForegroundColor Green
Write-Host '  curl http://192.168.8.21:11434/api/tags'
Write-Host '  myagent --provider-url http://192.168.8.21:11434'

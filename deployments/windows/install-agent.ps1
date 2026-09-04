[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)] [string]$ApiUrl,
    [Parameter(Mandatory = $true)] [string]$EnrollmentToken,
    [string]$AgentPath = "$PSScriptRoot\cyverra-agent-windows-amd64.exe"
)

$ErrorActionPreference = 'Stop'
$serviceName = 'CyverraNexusAgent'
$stateDir = Join-Path $env:ProgramData 'Cyverra\Agent'
$statePath = Join-Path $stateDir 'agent.json'
$targetPath = Join-Path $stateDir 'cyverra-agent.exe'

if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run PowerShell as Administrator.'
}
if (-not (Test-Path $AgentPath)) { throw "Agent binary not found: $AgentPath" }
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
Copy-Item -Force $AgentPath $targetPath

if (-not (Test-Path $statePath)) {
    & $targetPath --api $ApiUrl --enrollment-token $EnrollmentToken --state $statePath --once
    if ($LASTEXITCODE -ne 0) { throw "Enrollment failed with exit code $LASTEXITCODE" }
}

$existing = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($existing) { Stop-Service $serviceName -Force -ErrorAction SilentlyContinue; sc.exe delete $serviceName | Out-Null; Start-Sleep -Seconds 1 }
New-Service -Name $serviceName -BinaryPathName "`"$targetPath`" --api `"$ApiUrl`" --state `"$statePath`"" -DisplayName 'Cyverra Nexus Endpoint Agent' -Description 'Cyverra Nexus UEM endpoint management agent' -StartupType Automatic | Out-Null
Start-Service $serviceName
Write-Host "Cyverra agent installed and running as $serviceName"
Write-Host "Log: $statePath.log"

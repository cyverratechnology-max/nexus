$ErrorActionPreference = 'Stop'
$serviceName = 'CyverraNexusAgent'
$service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
if ($service) { Stop-Service $serviceName -Force -ErrorAction SilentlyContinue; sc.exe delete $serviceName | Out-Null }
Write-Host 'Cyverra Nexus agent service removed. State and logs were preserved under C:\ProgramData\Cyverra\Agent.'

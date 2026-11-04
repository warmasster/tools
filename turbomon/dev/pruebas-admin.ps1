# Pruebas que necesitan permiso de administrador: eventos de fotogramas, red,
# archivos, arranque de procesos y drivers (DPC e interrupciones). Solo leen
# eventos de Windows: no cambian ningún ajuste.
#
#   Start-Process powershell -Verb RunAs -ArgumentList '-ExecutionPolicy Bypass -File dev\pruebas-admin.ps1 -Log C:\ruta\pruebas.log'
param([string]$Log = "$env:TEMP\turbomon-pruebas-admin.log", [string]$Only = "")

$ErrorActionPreference = "Continue"
Set-Location (Split-Path $PSScriptRoot -Parent)
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:Path"
$env:TURBOMON_REAL = "1"
$out = @()
foreach ($t in @(
        @("RealETW", "./internal/fps/"),
        @("RealNetwork", "./internal/netmon/"),
        @("Real", "./internal/prefetch/"),
        @("RealDeep", "./internal/noise/"))) {
    if ($Only -and $t[1] -notmatch $Only) { continue }
    $out += "===== $($t[1]) -run $($t[0]) ====="
    $out += (go test -count=1 -run $t[0] -v $t[1] 2>&1 | Out-String)
}
$out | Set-Content -Encoding utf8 $Log
"hecho" | Add-Content -Encoding utf8 $Log

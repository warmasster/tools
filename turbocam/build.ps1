# Compila TurboCAM.exe (un único ejecutable, sin dependencias) y lo deja en esta carpeta.
# Primero la cámara virtual en C (camera\build.ps1), que va dentro del .exe.
$ErrorActionPreference = "Stop"
# Ojo: "Stop" no para con los programas (go, wails) que fallan; por eso cada paso
# mira $LASTEXITCODE.
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"

& .\camera\build.ps1
Set-Location $PSScriptRoot # el de la cámara cambia de carpeta
go vet .
if ($LASTEXITCODE) { throw "go vet falló" }
go test -short .
if ($LASTEXITCODE) { throw "Las pruebas fallaron" }
wails build -skipbindings -trimpath -ldflags "-s -w"
if ($LASTEXITCODE) { throw "wails build falló" }
Copy-Item .\build\bin\TurboCAM.exe .\TurboCAM.exe -Force
Write-Host "Listo: $(Resolve-Path .\TurboCAM.exe)"

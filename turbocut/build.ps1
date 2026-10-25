# Compila TurboCUT.exe (un único ejecutable, sin dependencias) y lo copia a la raíz del proyecto.
$ErrorActionPreference = "Stop"
# Ojo: "Stop" no para con los programas (go, wails) que fallan; por eso cada paso
# mira $LASTEXITCODE.
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"

go vet .
if ($LASTEXITCODE) { throw "go vet falló" }
go test -short .
if ($LASTEXITCODE) { throw "Las pruebas fallaron" }
wails build -skipbindings -trimpath -ldflags "-s -w"
if ($LASTEXITCODE) { throw "wails build falló" }
Copy-Item .\build\bin\TurboCUT.exe .\TurboCUT.exe -Force
Write-Host "Listo: $(Resolve-Path .\TurboCUT.exe)"

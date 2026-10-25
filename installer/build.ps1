# Compila TurboSETUP.exe (un único ejecutable, sin dependencias) y lo deja en esta carpeta.
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
Copy-Item .\build\bin\TurboSETUP.exe .\TurboSETUP.exe -Force
Write-Host "Listo: $(Resolve-Path .\TurboSETUP.exe)"

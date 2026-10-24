# Compila TurboVAULT.exe (un único ejecutable, con la extensión de Chrome dentro).
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"

go vet ./...
if ($LASTEXITCODE) { throw "go vet falló" }
go test -short ./...
if ($LASTEXITCODE) { throw "Las pruebas fallaron" }
wails build -skipbindings -trimpath -ldflags "-s -w"
if ($LASTEXITCODE) { throw "wails build falló" }
Copy-Item .\build\bin\TurboVAULT.exe .\TurboVAULT.exe -Force
Write-Host "Listo: $(Resolve-Path .\TurboVAULT.exe)"

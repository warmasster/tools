# Compila TurboSEND.exe (un único ejecutable, sin dependencias) y lo copia a la raíz del proyecto.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"

go vet .
go test -short .
wails build -skipbindings -trimpath -ldflags "-s -w"
Copy-Item .\build\bin\TurboSEND.exe ..\TurboSEND.exe -Force
Write-Host "Listo: $(Resolve-Path ..\TurboSEND.exe)"

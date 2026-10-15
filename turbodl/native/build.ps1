# Compila turbodl-native.exe (motor nativo de TurboDL, un único ejecutable sin dependencias).
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"

go vet .
go test -short .
go build -trimpath -ldflags "-s -w" -o turbodl-native.exe .
Write-Host "Listo: $(Resolve-Path .\turbodl-native.exe)"
Write-Host "Para instalarlo: doble clic en turbodl-native.exe (o: .\turbodl-native.exe install)"

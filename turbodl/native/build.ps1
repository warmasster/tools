# Compila TurboDL.exe: un único ejecutable que lleva dentro la extensión y el motor
# nativo. Doble clic = instala el motor y deja la extensión lista para cargar en Chrome.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"

# 1. Copiar la extensión a ext\ (go:embed la mete en el ejecutable).
Get-ChildItem ext -Exclude LEEME.txt -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
$root = Resolve-Path ..
foreach ($f in @("manifest.json", "background.js", "download-worker.js", "offscreen.html", "offscreen.js", "popup.html", "manager.html")) {
    Copy-Item (Join-Path $root $f) ext\
}
foreach ($d in @("lib", "ui", "icons")) {
    Copy-Item (Join-Path $root $d) ext\ -Recurse
}
$version = (Get-Content ..\manifest.json -Raw | ConvertFrom-Json).version

# 2. Icono, versión y manifiesto de Windows (.syso), sin descargar nada.
Push-Location tools\winres
go run . -icons ..\..\..\icons -out ..\..\rsrc_windows_amd64.syso -version $version
Pop-Location

# 3. Comprobar y compilar.
go vet .
go test -short .
go build -trimpath -ldflags "-s -w -X main.version=$version" -o ..\TurboDL.exe .
Write-Host "Listo: $(Resolve-Path ..\TurboDL.exe) (versión $version)"
Write-Host "Doble clic en TurboDL.exe para instalar."

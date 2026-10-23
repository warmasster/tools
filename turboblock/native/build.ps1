# Compila TurboBLOCK.exe: un único ejecutable con la extensión dentro (y las listas de
# filtros ya compiladas), el actualizador de listas y el instalador de doble clic.
#   .\build.ps1             descarga las listas que hayan caducado y compila
#   .\build.ps1 -Offline    solo con las listas de la caché (native\.cache)
param([switch]$Offline)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"
$root = Resolve-Path ..
$version = (Get-Content ..\manifest.json -Raw | ConvertFrom-Json).version

# 1. Comprobar el código y compilar las listas en ..\rules (también sirve para cargar
#    la extensión desde la carpeta del repositorio).
go vet .
if ($LASTEXITCODE) { throw "go vet falló" }
go test -short .
if ($LASTEXITCODE) { throw "Las pruebas fallaron" }
$compileArgs = @("run", ".", "compile", "--ext", "..", "--cache", ".cache")
if ($Offline) { $compileArgs += "--offline" }
& go @compileArgs
if ($LASTEXITCODE) { throw "No se pudieron compilar las listas" }

# 2. La extensión, comprimida, a ext\extension.zip (go:embed la mete en el .exe).
Get-ChildItem ext -Exclude LEEME.txt -ErrorAction SilentlyContinue | Remove-Item -Recurse -Force
$stage = Join-Path ([IO.Path]::GetTempPath()) "turboblock-build"
if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
New-Item -ItemType Directory $stage | Out-Null
foreach ($f in @("manifest.json", "background.js", "popup.html", "options.html", "blocked.html")) {
    Copy-Item (Join-Path $root $f) $stage
}
foreach ($d in @("lib", "content", "ui", "icons", "resources", "rules", "filters")) {
    Copy-Item (Join-Path $root $d) $stage -Recurse
}
Add-Type -AssemblyName System.IO.Compression.FileSystem
[IO.Compression.ZipFile]::CreateFromDirectory($stage, (Join-Path $PSScriptRoot "ext\extension.zip"), [IO.Compression.CompressionLevel]::Optimal, $false)
Remove-Item $stage -Recurse -Force

# 3. Icono, versión y manifiesto de Windows (.syso), sin descargar nada.
Push-Location tools\winres
go run . -icons ..\..\..\icons -out ..\..\rsrc_windows_amd64.syso -version $version
Pop-Location
if ($LASTEXITCODE) { throw "No se pudo generar el icono" }

# 4. Compilar.
go build -trimpath -ldflags "-s -w -X main.version=$version" -o ..\TurboBLOCK.exe .
if ($LASTEXITCODE) { throw "go build falló" }
$size = [math]::Round((Get-Item ..\TurboBLOCK.exe).Length / 1MB, 1)
Write-Host "Listo: $(Resolve-Path ..\TurboBLOCK.exe) (versión $version, $size MB)"
Write-Host "Doble clic en TurboBLOCK.exe para instalar."

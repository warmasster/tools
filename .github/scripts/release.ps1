# Publica en la Release "apps" lo que haya cambiado de versión.
#
# Lee tools.json, compara la versión de cada programa con la del manifest.json
# publicado y solo compila y sube los que han cambiado. Al final sube el
# manifest.json nuevo, que es lo que lee TurboSETUP.
#
#   .github/scripts/release.ps1            # en GitHub Actions (usa gh)
#   .github/scripts/release.ps1 -Force     # recompila y sube todo
#   .github/scripts/release.ps1 -OutDir dist   # en local: deja todo en dist\ sin subir nada
param(
    [switch]$Force,
    [string]$OutDir = ""
)
$ErrorActionPreference = "Stop"
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $root
$tag = "apps"
$local = $OutDir -ne ""

function Read-Version($file) {
    $j = Get-Content $file -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($j.info -and $j.info.productVersion) { return $j.info.productVersion }
    return $j.version
}

function Write-Utf8($path, $text) {
    [IO.File]::WriteAllText($path, $text, (New-Object Text.UTF8Encoding $false))
}

function Ensure-Wails {
    if (Get-Command wails -ErrorAction SilentlyContinue) { return }
    Write-Host "Instalando Wails..."
    go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 | Out-Host
    $env:Path = "$(go env GOPATH)\bin;$env:Path"
}

# Compila con el build.ps1 de cada programa y devuelve los datos del .exe.
function Build-One($entry, $version) {
    Write-Host "`n=== $($entry.name) $version ==="
    Ensure-Wails
    & (Join-Path $root $entry.build) | Out-Host
    if ($LASTEXITCODE) { throw "Falló la compilación de $($entry.name)" }
    $exe = Join-Path $root $entry.output
    $item = Get-Item $exe
    return [ordered]@{
        version = $version
        asset   = $item.Name
        size    = $item.Length
        sha256  = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLower()
        path    = $exe
    }
}

# --- manifest publicado ahora mismo ---
$old = $null
$work = Join-Path ([IO.Path]::GetTempPath()) "turbo-release"
New-Item -ItemType Directory -Force $work | Out-Null
if ($local) {
    New-Item -ItemType Directory -Force $OutDir | Out-Null
    $OutDir = Resolve-Path $OutDir
    $prev = Join-Path $OutDir "manifest.json"
    if (Test-Path $prev) { $old = Get-Content $prev -Raw | ConvertFrom-Json }
} else {
    gh release view $tag *> $null
    if ($LASTEXITCODE) {
        Write-Host "Creando la Release '$tag'..."
        gh release create $tag --title "TurboSETUP" --notes "Descarga TurboSETUP.exe" --latest
        if ($LASTEXITCODE) { throw "No se pudo crear la Release" }
    } else {
        gh release download $tag --pattern manifest.json --dir $work --clobber 2>$null
        $prev = Join-Path $work "manifest.json"
        if (Test-Path $prev) { $old = Get-Content $prev -Raw | ConvertFrom-Json }
    }
}

$cat = Get-Content (Join-Path $root "tools.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$uploads = @()

function Pick($entry, $prevAsset) {
    $v = Read-Version (Join-Path $root $entry.version)
    if (-not $Force -and $prevAsset -and $prevAsset.version -eq $v -and $prevAsset.sha256) {
        Write-Host "$($entry.name) $v sin cambios"
        return [ordered]@{ version = $v; asset = $prevAsset.asset; size = $prevAsset.size; sha256 = $prevAsset.sha256 }
    }
    $b = Build-One $entry $v
    $script:uploads += $b.path
    $b.Remove("path")
    return $b
}

$setup = Pick $cat.setup ($old.setup)
$tools = @()
foreach ($t in $cat.tools) {
    $prevT = $old.tools | Where-Object { $_.id -eq $t.id } | Select-Object -First 1
    $a = Pick $t $prevT
    $tools += [ordered]@{
        id = $t.id; name = $t.name; summary = $t.summary; kind = $t.kind; dir = $t.dir
        version = $a.version; asset = $a.asset; size = $a.size; sha256 = $a.sha256
    }
}

if ($uploads.Count -eq 0 -and -not $Force) {
    Write-Host "`nNada que publicar: ninguna versión ha cambiado."
    exit 0
}

$manifest = [ordered]@{
    updated = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    setup   = $setup
    tools   = $tools
}
$manifestPath = Join-Path $work "manifest.json"
Write-Utf8 $manifestPath ($manifest | ConvertTo-Json -Depth 5)

# Notas de la Release: qué versión hay de cada cosa.
$notes = @(
    "Descarga **TurboSETUP.exe** y elige qué instalar. Se encarga también de las actualizaciones.",
    "",
    "| Programa | Versión |",
    "|---|---|",
    "| TurboSETUP | $($setup.version) |"
) + ($tools | ForEach-Object { "| $($_.name) | $($_.version) |" })
$notesPath = Join-Path $work "notes.md"
Write-Utf8 $notesPath ($notes -join "`n")

if ($local) {
    foreach ($u in $uploads) { Copy-Item $u $OutDir -Force }
    Copy-Item $manifestPath $OutDir -Force
    Write-Host "`nListo en $OutDir"
    exit 0
}

# Primero los .exe y después el manifest: así nunca anuncia algo que aún no está subido.
foreach ($u in $uploads) {
    gh release upload $tag $u --clobber
    if ($LASTEXITCODE) { throw "No se pudo subir $u" }
}
gh release upload $tag $manifestPath --clobber
if ($LASTEXITCODE) { throw "No se pudo subir manifest.json" }
gh release edit $tag --notes-file $notesPath
Write-Host "`nPublicado: $($uploads.Count) programa(s) nuevo(s)."

# Publica en la Release "apps" lo que haya cambiado de versión.
#
# Lee tools.json, compara la versión de cada programa con la del manifest.json
# publicado y solo compila y sube los que han cambiado. Al final sube el
# manifest.json nuevo, que es lo que lee TurboSETUP.
#
# Cada .exe se sube dos veces, siempre antes que el manifest: con la versión en el
# nombre (TurboSEND-2.1.2.exe, sin sustituir nunca nada: es lo que baja TurboSETUP
# 1.0.7 o posterior) y con su nombre de siempre (para el enlace del README y los
# TurboSETUP de antes). Así el manifest nunca anuncia un archivo a medio subir, y si
# algo falla antes de subirlo, la siguiente ejecución lo vuelve a intentar todo.
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
    # En un proceso aparte para quedarse con todo lo que escribe aunque falle. En
    # GitHub, las líneas del error salen como avisos del workflow, que se ven sin
    # iniciar sesión (el registro completo no).
    $shell = (Get-Process -Id $PID).Path
    $out = & $shell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root $entry.build) 2>&1 | Out-String
    $code = $LASTEXITCODE
    Set-Location $root
    Write-Host $out
    if ($code) {
        if ($env:GITHUB_ACTIONS) {
            # GitHub enseña como mucho 10 avisos por paso: primero un resumen con todo.
            $all = $out -split "`r?`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ }
            $pkgs = $all | Where-Object { $_ -match '^FAIL\s+\S' } | ForEach-Object { ($_ -split '\s+')[1] }
            $tests = $all | Where-Object { $_ -match '^--- FAIL' } | ForEach-Object { ($_ -split '\s+')[2] }
            if ($pkgs) { Write-Host "::error::$($entry.name): paquetes con fallos: $($pkgs -join ' · ')" }
            if ($tests) { Write-Host "::error::$($entry.name): pruebas que fallan: $($tests -join ' · ')" }
            $detail = $all | Where-Object { $_ -match '_test\.go:\d+|panic|falló|fallaron|error|Error|\.go:\d+:\d+' } | Select-Object -First 8
            foreach ($l in $detail) { Write-Host "::error::$($entry.name): $l" }
        }
        throw "Falló la compilación de $($entry.name)"
    }
    $exe = Join-Path $root $entry.output
    $item = Get-Item $exe
    return [ordered]@{
        version  = $version
        asset    = $item.Name
        size     = $item.Length
        sha256   = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLower()
        download = Versioned-Name $item.Name $version
        path     = $exe
    }
}

# TurboSEND.exe + 2.1.2 -> TurboSEND-2.1.2.exe
function Versioned-Name($asset, $version) {
    return [IO.Path]::GetFileNameWithoutExtension($asset) + "-" + $version + [IO.Path]::GetExtension($asset)
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
    # Solo existe la Release "apps": cualquier otra (de antes de TurboSETUP) se borra con su etiqueta.
    $others = gh release list --limit 100 --json tagName --jq '.[].tagName' | Where-Object { $_ -and $_ -ne $tag }
    foreach ($o in $others) {
        Write-Host "Borrando la Release vieja '$o'..."
        gh release delete $o --yes --cleanup-tag
    }

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
        $same = [ordered]@{ version = $v; asset = $prevAsset.asset; size = $prevAsset.size; sha256 = $prevAsset.sha256 }
        # Publicado antes de los nombres con versión: sigue sin él hasta su próxima versión.
        if ($prevAsset.download) { $same.download = $prevAsset.download }
        return $same
    }
    $b = Build-One $entry $v
    $script:uploads += [pscustomobject]@{ path = $b.path; download = $b.download }
    $b.Remove("path")
    return $b
}

$setup = Pick $cat.setup ($old.setup)
$tools = @()
foreach ($t in $cat.tools) {
    $prevT = $old.tools | Where-Object { $_.id -eq $t.id } | Select-Object -First 1
    $a = Pick $t $prevT
    $row = [ordered]@{
        id = $t.id; name = $t.name; summary = $t.summary; kind = $t.kind; open = [bool]$t.open; dir = $t.dir
        version = $a.version; asset = $a.asset; size = $a.size; sha256 = $a.sha256
    }
    if ($a.download) { $row.download = $a.download }
    $tools += $row
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

# Copias con la versión en el nombre, en la carpeta de trabajo.
foreach ($u in $uploads) {
    $u | Add-Member -NotePropertyName versioned -NotePropertyValue (Join-Path $work $u.download) -Force
    Copy-Item $u.path $u.versioned -Force
}

if ($local) {
    foreach ($u in $uploads) {
        Copy-Item $u.path $OutDir -Force
        Copy-Item $u.versioned $OutDir -Force
    }
    Copy-Item $manifestPath $OutDir -Force
    Write-Host "`nListo en $OutDir"
    exit 0
}

# 1. Los .exe con versión: nombres nuevos, no pisan nada que se esté descargando.
foreach ($u in $uploads) {
    gh release upload $tag $u.versioned --clobber
    if ($LASTEXITCODE) { throw "No se pudo subir $($u.download)" }
}
# 2. Los de nombre fijo (README y TurboSETUP de antes de la 1.0.7). Los nuevos no los
# usan, así que cambiarlos ahora no les afecta.
foreach ($u in $uploads) {
    gh release upload $tag $u.path --clobber
    if ($LASTEXITCODE) { throw "No se pudo subir $($u.path)" }
}
# 3. El manifest, el último: desde aquí TurboSETUP ve la versión nueva, ya entera.
gh release upload $tag $manifestPath --clobber
if ($LASTEXITCODE) { throw "No se pudo subir manifest.json" }
gh release edit $tag --notes-file $notesPath

# 4. Se borran las copias con versión de hace más de una publicación (la anterior se
# queda: alguien con el catálogo viejo puede estar bajándola ahora mismo).
$keep = @{}
foreach ($a in @($setup) + $tools) { if ($a.download) { $keep[$a.download] = $true } }
foreach ($a in @($old.setup) + @($old.tools)) { if ($a -and $a.download) { $keep[$a.download] = $true } }
$stems = (@($setup) + $tools) | ForEach-Object { [regex]::Escape([IO.Path]::GetFileNameWithoutExtension($_.asset)) }
$pattern = '^(' + ($stems -join '|') + ')-\d[0-9A-Za-z.]*\.exe$'
$names = gh release view $tag --json assets --jq '.assets[].name'
foreach ($n in $names) {
    if ($n -match $pattern -and -not $keep[$n]) {
        Write-Host "Borrando la copia vieja $n"
        gh release delete-asset $tag $n --yes
        if ($LASTEXITCODE) { Write-Host "No se pudo borrar $n (no pasa nada: se intenta en la próxima publicación)" }
    }
}
Write-Host "`nPublicado: $($uploads.Count) programa(s) nuevo(s)."

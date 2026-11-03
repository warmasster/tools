# Publica lo que haya cambiado de versión.
#
# Lee tools.json, compara la versión de cada programa con la del manifest.json
# publicado y solo compila y sube los que han cambiado. Hay dos tipos de Release:
#
#   - Una por versión de cada programa (turbocam-v1.0.6, «TurboCAM 1.0.6»), con sus
#     novedades (sacadas de los commits) y su .exe. No se toca nunca una vez publicada:
#     de aquí baja TurboSETUP 1.0.9 o posterior, así que lo que anuncia el catálogo
#     siempre está entero aunque se esté publicando otra versión.
#   - "apps", la de siempre: TurboSETUP.exe (el enlace del README), el .exe de la
#     última versión de cada programa con su nombre de siempre (para bajarlo suelto y
#     para los TurboSETUP de antes) y manifest.json, el catálogo. Se sube lo último.
#
# Si un programa ya publicado no tiene todavía su Release de versión (las de antes de
# este sistema), se le crea con el mismo .exe que hay en "apps", sin recompilarlo.
#
#   .github/scripts/release.ps1            # en GitHub Actions (usa gh)
#   .github/scripts/release.ps1 -Force     # recompila y sube todo
#   .github/scripts/release.ps1 -OutDir dist   # en local: deja todo en dist\ sin subir nada
param(
    [switch]$Force,
    [string]$OutDir = ""
)
$ErrorActionPreference = "Stop"
# Cualquier error sale como aviso del workflow, que se ve sin iniciar sesión.
trap {
    $msg = "línea $($_.InvocationInfo.ScriptLineNumber): $($_.Exception.Message)" -replace '\r?\n', ' · '
    if ($env:GITHUB_ACTIONS) { Write-Host "::error::release.ps1, $msg" } else { Write-Host $msg }
    exit 1
}
try { [Console]::OutputEncoding = New-Object Text.UTF8Encoding $false } catch {}
$OutputEncoding = New-Object Text.UTF8Encoding $false
$root = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $root
$tag = "apps"
$web = "https://github.com/warmasster/tools"
$setupUrl = "$web/releases/download/$tag/TurboSETUP.exe"
$local = $OutDir -ne ""

function Parse-Version($raw) {
    $j = $raw.TrimStart([char]0xFEFF) | ConvertFrom-Json
    if ($j.info -and $j.info.productVersion) { return $j.info.productVersion }
    return $j.version
}

function Read-Version($file) {
    return Parse-Version (Get-Content $file -Raw -Encoding UTF8)
}

function Write-Utf8($path, $text) {
    [IO.File]::WriteAllText($path, $text, (New-Object Text.UTF8Encoding $false))
}

# git sin que un aviso por stderr pare el script (Windows PowerShell lo trata como error).
function Run-Git {
    $ErrorActionPreference = "Continue"
    $out = & git -c core.quotepath=off @args 2>$null
    if ($LASTEXITCODE) { return @() }
    return @($out)
}

# gh que, si falla, para el script con lo que ha respondido. Devuelve lo que escribe.
function Invoke-Gh {
    $ErrorActionPreference = "Continue"
    $out = @(& gh @args 2>&1)
    if ($LASTEXITCODE) { throw "gh $($args -join ' '): $(($out | Out-String).Trim())" }
    return $out | Where-Object { $_ -isnot [System.Management.Automation.ErrorRecord] } | ForEach-Object { "$_" }
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
        version = $version
        asset   = $item.Name
        size    = $item.Length
        sha256  = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLower()
        path    = $exe
    }
}

# --- Historia de versiones y novedades, a partir de git ---

# Tramos de la historia del archivo de versión, del más nuevo al más viejo. «first»
# es el commit que puso esa versión.
function Version-Runs($file) {
    $runs = @()
    $cur = $null
    foreach ($c in (Run-Git log --format=%H -- $file)) {
        $raw = (Run-Git show "${c}:$file") -join "`n"
        $v = $null
        if ($raw) { try { $v = Parse-Version $raw } catch {} }
        if ($cur -and $cur.version -eq $v) { $cur.first = $c; continue }
        $cur = [pscustomobject]@{ version = $v; first = $c }
        $runs += $cur
    }
    return $runs
}

# Qué commits van en las novedades de la versión $v: desde el que puso la versión
# anterior hasta $end. Con -Backfill (Release de una versión ya publicada), hasta el
# commit que puso esta versión, que es también donde se pone la etiqueta.
function Version-Range($file, $v, [switch]$Backfill) {
    $runs = @(Version-Runs $file)
    $head = (Run-Git rev-parse HEAD) | Select-Object -First 1
    $i = -1
    for ($k = 0; $k -lt $runs.Count; $k++) { if ($runs[$k].version -eq $v) { $i = $k; break } }
    if ($i -lt 0) {
        # Versión subida sin commit todavía (en local).
        $start = if ($runs.Count) { $runs[0].first } else { $null }
        return [pscustomobject]@{ start = $start; end = $head; prev = $(if ($runs.Count) { $runs[0].version }) }
    }
    $start = $null; $prev = $null
    if ($i + 1 -lt $runs.Count) { $start = $runs[$i + 1].first; $prev = $runs[$i + 1].version }
    $end = if ($Backfill) { $runs[$i].first } else { $head }
    return [pscustomobject]@{ start = $start; end = $end; prev = $prev }
}

# Archivos que no cambian nada para quien usa el programa.
function Is-DevFile($f) {
    return $f -match '_test\.go$|/testdata/|/dev/|(^|/)README\.md$|(^|/)build\.ps1$|\.gitignore$'
}

# Las viñetas de las novedades: el cuerpo de cada commit del programa (o su asunto si
# no tiene cuerpo), y de los commits que tocan varios programas («Turbo: …»), solo
# las viñetas que empiezan por su nombre. Lo que es solo de pruebas no sale.
function Release-Items($name, $dir, $range) {
    $span = if ($range.start) { "$($range.start)..$($range.end)" } else { $range.end }
    $log = (Run-Git log "--format=%H%x1f%s%x1f%b%x1e" $span -- "$dir/") -join "`n"
    $items = New-Object System.Collections.Generic.List[string]
    $nameRe = [regex]::Escape($name)
    foreach ($rec in ($log -split [char]0x1e)) {
        $p = $rec.Trim() -split [char]0x1f
        if ($p.Count -lt 2) { continue }
        $hash = $p[0].Trim()
        $subj = $p[1].Trim()
        $body = if ($p.Count -gt 2) { $p[2] } else { "" }
        $files = Run-Git diff-tree --no-commit-id --name-only -r $hash -- "$dir/"
        if (-not ($files | Where-Object { -not (Is-DevFile $_) })) { continue }

        $own = $null
        if ($subj -match "^$nameRe\b[^:]*:\s*(.+)$") { $own = $Matches[1] }
        $found = @()
        foreach ($line in ($body -split "\r?\n")) {
            if ($line -notmatch '^\s*[-*]\s+(.+)$') { continue }
            $text = $Matches[1].Trim()
            if ($text -match '^(Turbo[A-Za-z]+)\b[^:]{0,20}:\s*(.+)$') {
                # Viñeta con nombre: solo si es de este programa.
                if ($Matches[1] -eq $name) { $found += $Matches[2] }
            } elseif ($own) {
                $found += $text
            }
        }
        if ($own -and -not $found) { $found = @($own) }
        foreach ($t in $found) {
            $t = $t.Trim()
            if (-not $t -or $t -match '\bpruebas?\b|\bTURBO[A-Z]+_[A-Z_]+\b|\bgo (test|vet)\b') { continue }
            # «Versión 1.0.5», «Extensión 1.0.5 y app 1.0.6»: ya lo dice el título.
            if ($t -match '^((versi[oó]n|extensi[oó]n|app)\s+\d+\.\d+\S*(\s+y\s+)?)+\.?$') { continue }
            $t = $t.Substring(0, 1).ToUpper() + $t.Substring(1)
            if ($t -notmatch '[.!?…]$') { $t += "." }
            # Que GitHub enseñe tal cual las rutas (C:\Windows) y no tome <id> por HTML.
            $t = $t.Replace("\", "\\").Replace("<", "&lt;").Replace(">", "&gt;")
            if (-not $items.Contains($t)) { $items.Add($t) }
        }
    }
    if ($items.Count -eq 0) { $items.Add("Mejoras internas y correcciones.") }
    return $items
}

function Tool-Notes($e, $v, $asset, $sha, $relTag, $range) {
    $items = Release-Items $e.name $e.dir $range
    $l = @()
    if ($e.summary) { $l += "> $($e.summary)", "" }
    $l += "### Novedades"
    $l += $items | ForEach-Object { "- $_" }
    $l += "", "### Descargar"
    if ($e.id -eq "turbosetup") {
        $l += "Baja **$asset** aquí abajo, ábrelo y elige qué herramientas quieres. La última versión está siempre en [este enlace]($setupUrl)."
    } else {
        $l += "Lo más fácil es instalarla con **[TurboSETUP]($setupUrl)**, que también avisa cuando hay versión nueva. Si lo prefieres, baja **$asset** aquí abajo."
    }
    $foot = @()
    $prevTag = "$($e.id)-v$($range.prev)"
    if ($range.prev -and $script:releases[$prevTag]) { $foot += "Versión anterior: [$($range.prev)]($web/releases/tag/$prevTag)" }
    $foot += "Código: [$($e.dir)/]($web/tree/$relTag/$($e.dir))"
    $foot += "SHA-256: ``$sha``"
    $l += "", ("<sub>" + ($foot -join " · ") + "</sub>")
    return ($l -join "`n")
}

# --- Lo publicado ahora mismo ---
$old = $null
$work = Join-Path ([IO.Path]::GetTempPath()) "turbo-release"
if (Test-Path $work) { Remove-Item -Recurse -Force $work }
New-Item -ItemType Directory -Force $work | Out-Null
$releases = @{}
$cat = Get-Content (Join-Path $root "tools.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$ids = @("turbosetup") + ($cat.tools | ForEach-Object { $_.id })
$relPattern = '^(' + (($ids | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')-v\d[0-9A-Za-z.]*$'

if ($local) {
    New-Item -ItemType Directory -Force $OutDir | Out-Null
    $OutDir = Resolve-Path $OutDir
    $prev = Join-Path $OutDir "manifest.json"
    if (Test-Path $prev) { $old = Get-Content $prev -Raw | ConvertFrom-Json }
    Get-ChildItem $OutDir -Directory | Where-Object { $_.Name -match $relPattern } | ForEach-Object { $releases[$_.Name] = $true }
} else {
    foreach ($r in (gh release list --limit 1000 --json tagName --jq '.[].tagName')) {
        if (-not $r) { continue }
        if ($r -eq $tag -or $r -match $relPattern) { $releases[$r] = $true; continue }
        # Cualquier otra Release (de antes de TurboSETUP, o hecha a mano) se borra con su etiqueta.
        Write-Host "Borrando la Release '$r', que no es de ningún programa..."
        gh release delete $r --yes --cleanup-tag
    }
    if (-not $releases[$tag]) {
        Write-Host "Creando la Release '$tag'..."
        gh release create $tag --title "TurboSETUP" --notes "Descarga TurboSETUP.exe" --latest
        if ($LASTEXITCODE) { throw "No se pudo crear la Release" }
    } else {
        gh release download $tag --pattern manifest.json --dir $work --clobber 2>$null
        $prev = Join-Path $work "manifest.json"
        if (Test-Path $prev) { $old = Get-Content $prev -Raw | ConvertFrom-Json }
    }
}

# Releases de versión por crear o rellenar: { entry; version; tag; path; sha; target; notes; built }
$uploads = @()

function Pick($e) {
    $v = Read-Version (Join-Path $root $e.version)
    $relTag = "$($e.id)-v$v"
    $p = $e.prev
    if (-not $Force -and $p -and $p.version -eq $v -and $p.sha256) {
        Write-Host "$($e.name) $v sin cambios"
        $same = [ordered]@{ version = $v; asset = $p.asset; size = $p.size; sha256 = $p.sha256 }
        if ($script:releases[$relTag]) {
            $same.release = $relTag
        } elseif (-not $local) {
            # Publicado antes de las Releases por versión: se le crea la suya con el .exe de "apps".
            $dir = Join-Path $work "apps-$($e.id)"
            New-Item -ItemType Directory -Force $dir | Out-Null
            $ok = $true
            try { $null = Invoke-Gh release download $tag --pattern $p.asset --dir $dir --clobber } catch { $ok = $false; Write-Host "::warning::$($_.Exception.Message)" }
            $file = Join-Path $dir $p.asset
            if ($ok -and (Test-Path $file) -and (Get-FileHash $file -Algorithm SHA256).Hash.ToLower() -eq $p.sha256) {
                $range = Version-Range $e.version $v -Backfill
                $script:uploads += [pscustomobject]@{
                    entry = $e; version = $v; tag = $relTag; path = $file; target = $range.end; built = $false
                    notes = Tool-Notes $e $v $p.asset $p.sha256 $relTag $range
                }
                $same.release = $relTag
            } else {
                Write-Host "::warning::$($e.name): el $($p.asset) de '$tag' no coincide con el catálogo; se queda sin Release de versión hasta la próxima."
            }
        }
        return $same
    }
    $b = Build-One $e $v
    $range = Version-Range $e.version $v
    $script:uploads += [pscustomobject]@{
        entry = $e; version = $v; tag = $relTag; path = $b.path; target = $range.end; built = $true
        notes = Tool-Notes $e $v $b.asset $b.sha256 $relTag $range
    }
    $b.Remove("path")
    $b.release = $relTag
    return $b
}

$entries = @([pscustomobject]@{
        id = "turbosetup"; name = $cat.setup.name; dir = "installer"; build = $cat.setup.build; output = $cat.setup.output
        version = $cat.setup.version; prev = $old.setup
        summary = "Instala y actualiza todas las herramientas Turbo: eliges cuáles quieres y se encarga del resto."
    })
foreach ($t in $cat.tools) {
    $entries += [pscustomobject]@{
        id = $t.id; name = $t.name; dir = $t.dir; build = $t.build; output = $t.output; version = $t.version
        prev = ($old.tools | Where-Object { $_.id -eq $t.id } | Select-Object -First 1); summary = $t.summary; t = $t
    }
}

$setup = Pick $entries[0]
$tools = @()
foreach ($e in ($entries | Select-Object -Skip 1)) {
    $t = $e.t
    $a = Pick $e
    $row = [ordered]@{
        id = $t.id; name = $t.name; summary = $t.summary; kind = $t.kind; open = [bool]$t.open; dir = $t.dir
        version = $a.version; asset = $a.asset; size = $a.size; sha256 = $a.sha256
    }
    if ($a.release) { $row.release = $a.release }
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

# Notas de "apps": la descarga de TurboSETUP y qué versión hay de cada cosa.
function Version-Cell($a) {
    if ($a.release) { return "[$($a.version)]($web/releases/tag/$($a.release))" }
    return $a.version
}
$notes = @(
    "## [⬇ Descargar TurboSETUP.exe]($setupUrl)",
    "",
    "Ábrelo, elige qué herramientas quieres y las instala. También se encarga de las actualizaciones.",
    "",
    "| Herramienta | Qué hace | Versión |",
    "|---|---|---|",
    "| **TurboSETUP** | $($entries[0].summary) | $(Version-Cell $setup) |"
) + ($tools | ForEach-Object { "| **$($_.name)** | $($_.summary) | $(Version-Cell $_) |" }) + @(
    "",
    "Las novedades de cada versión están en su propia Release (el enlace de la versión). Aquí abajo está siempre el ``.exe`` de la última versión de cada herramienta, por si prefieres bajarla suelta; ``manifest.json`` es el catálogo que lee TurboSETUP."
)
$notesPath = Join-Path $work "apps.md"
Write-Utf8 $notesPath ($notes -join "`n")

if ($local) {
    $notesDir = Join-Path $OutDir "notas"
    New-Item -ItemType Directory -Force $notesDir | Out-Null
    foreach ($u in $uploads) {
        $rel = Join-Path $OutDir $u.tag
        New-Item -ItemType Directory -Force $rel | Out-Null
        Copy-Item $u.path $rel -Force
        Copy-Item $u.path $OutDir -Force
        Write-Utf8 (Join-Path $notesDir "$($u.tag).md") $u.notes
    }
    Copy-Item $notesPath $notesDir -Force
    Copy-Item $manifestPath $OutDir -Force
    Write-Host "`nListo en $OutDir (las notas de cada Release, en notas\)"
    exit 0
}

# 1. Las Releases de versión. Al revés, para que en la lista salgan en el orden del
# catálogo (la más nueva arriba).
$order = @($uploads)
[array]::Reverse($order)
foreach ($u in $order) {
    $notesFile = Join-Path $work "notes-$($u.tag).md"
    Write-Utf8 $notesFile $u.notes
    $title = "$($u.entry.name) $($u.version)"
    if ($releases[$u.tag]) {
        # Solo con -Force o si una ejecución anterior se cortó antes del catálogo.
        $null = Invoke-Gh release upload $u.tag $u.path --clobber
        $null = Invoke-Gh release edit $u.tag --title $title --notes-file $notesFile --latest=false
    } else {
        Write-Host "Creando la Release $($u.tag) en $($u.target)..."
        $null = Invoke-Gh release create $u.tag $u.path --title $title --notes-file $notesFile --target $u.target --latest=false
    }
}
# 2. Los .exe con su nombre de siempre en "apps" (README y TurboSETUP de antes de la
# 1.0.9). Los nuevos no los usan, así que cambiarlos ahora no les afecta.
foreach ($u in ($uploads | Where-Object { $_.built })) {
    $null = Invoke-Gh release upload $tag $u.path --clobber
}
# 3. El catálogo, el último: desde aquí TurboSETUP ve la versión nueva, ya entera.
$null = Invoke-Gh release upload $tag $manifestPath --clobber
$null = Invoke-Gh release edit $tag --title "TurboSETUP · todas las herramientas" --notes-file $notesPath --latest

# 4. Las copias con la versión en el nombre (TurboCAM-1.0.6.exe) eran del sistema de
# antes; ya no las pide nadie que lea este catálogo.
$stems = (@($setup) + $tools) | ForEach-Object { [regex]::Escape([IO.Path]::GetFileNameWithoutExtension($_.asset)) }
$pattern = '^(' + ($stems -join '|') + ')-\d[0-9A-Za-z.]*\.exe$'
foreach ($n in (gh release view $tag --json assets --jq '.assets[].name')) {
    if ($n -match $pattern) {
        Write-Host "Borrando la copia vieja $n"
        gh release delete-asset $tag $n --yes
        if ($LASTEXITCODE) { Write-Host "No se pudo borrar $n (no pasa nada: se intenta en la próxima publicación)" }
    }
}
Write-Host "`nPublicado: $(@($uploads | Where-Object { $_.built }).Count) programa(s) nuevo(s), $(@($uploads | Where-Object { -not $_.built }).Count) Release(s) de versión de lo ya publicado."

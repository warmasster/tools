# Compila la cámara virtual (DLL de 64 y de 32 bits) en camera\bin\.
# TurboCAM.exe las lleva dentro; este script lo llama ..\build.ps1 antes de compilar Go.
#
# Necesita las herramientas de C++ de Visual Studio (Build Tools vale; en GitHub
# Actions ya vienen). Solo recompila si ha cambiado algo.
#
# CRT «híbrido»: el runtime de C va dentro de la DLL salvo la parte universal
# (ucrtbase.dll), que viene con Windows 10 y 11. Así no hace falta ningún
# redistribuible de Visual C++ en el PC de nadie.
param([switch]$Force)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$sources = @("turbocam.c", "protocol.h", "turbocam.def", "build.ps1")
$outs = @("bin\turbocam64.dll", "bin\turbocam32.dll")
$newest = ($sources | ForEach-Object { (Get-Item $_).LastWriteTimeUtc } | Measure-Object -Maximum).Maximum
$fresh = -not $Force
foreach ($o in $outs) {
    if (-not (Test-Path $o) -or (Get-Item $o).LastWriteTimeUtc -lt $newest) { $fresh = $false }
}
if ($fresh) {
    Write-Host "Cámara virtual: sin cambios"
    exit 0
}

$vswhere = Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio\Installer\vswhere.exe"
if (-not (Test-Path $vswhere)) { throw "No se encuentra Visual Studio (vswhere.exe): instala las Build Tools de C++" }
$vs = & $vswhere -latest -products * -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath
if (-not $vs) { throw "Faltan las herramientas de C++ de Visual Studio (x86/x64)" }
$vcvars = Join-Path $vs "VC\Auxiliary\Build\vcvarsall.bat"

New-Item -ItemType Directory -Force bin | Out-Null
$tmp = Join-Path ([IO.Path]::GetTempPath()) "turbocam-camera"

$cflags = "/nologo /c /O2 /GL /Gy /Gw /MT /W4 /WX /utf-8 /std:c17 /DUNICODE /D_UNICODE /DNDEBUG /guard:cf"
$libs = "strmiids.lib uuid.lib ole32.lib gdi32.lib user32.lib advapi32.lib kernel32.lib"
$lflags = "/nologo /DLL /DEF:turbocam.def /LTCG /OPT:REF /OPT:ICF /DYNAMICBASE /NXCOMPAT /guard:cf " +
          "/NODEFAULTLIB:libucrt.lib ucrt.lib /MANIFEST:NO /INCREMENTAL:NO"

foreach ($t in @(@{ arch = "x64"; out = "bin\turbocam64.dll" }, @{ arch = "x86"; out = "bin\turbocam32.dll" })) {
    $obj = Join-Path $tmp $t.arch
    New-Item -ItemType Directory -Force $obj | Out-Null
    # Un .cmd: PowerShell 5.1 estropea las comillas al pasárselas a cmd directamente.
    $bat = Join-Path $obj "build.cmd"
    $lines = @(
        "@echo off",
        "set `"PATH=$(Split-Path $vswhere);%PATH%`"",
        "call `"$vcvars`" $($t.arch) >nul || exit /b 1",
        "cl $cflags /Fo`"$obj\\`" turbocam.c || exit /b 1",
        "link $lflags /OUT:`"$($t.out)`" /IMPLIB:`"$obj\turbocam.lib`" `"$obj\turbocam.obj`" $libs || exit /b 1"
    )
    [IO.File]::WriteAllText($bat, ($lines -join "`r`n"), [Text.Encoding]::ASCII)
    cmd /d /c "`"$bat`""
    if ($LASTEXITCODE) { throw "No se pudo compilar la cámara virtual ($($t.arch))" }
}
Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
Get-Item $outs | ForEach-Object { Write-Host ("Cámara virtual: {0} ({1:N0} KB)" -f $_.Name, ($_.Length / 1KB)) }

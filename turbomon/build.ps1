# Compila TurboMON.exe (un único ejecutable: agente de la bandeja, overlay y ventana).
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$env:Path = "$env:LOCALAPPDATA\Programs\go\bin;$env:USERPROFILE\go\bin;$env:Path"

# Ejecuta un paso y, si falla en GitHub Actions, deja las líneas del error como
# avisos del workflow (se ven sin iniciar sesión, a diferencia del registro).
function Step($what, [scriptblock]$cmd) {
    $out = & $cmd 2>&1 | Out-String
    $code = $LASTEXITCODE
    Write-Host $out
    if ($code) {
        if ($env:GITHUB_ACTIONS) {
            $lines = $out -split "`r?`n" | Where-Object { $_ -match 'FAIL|_test\.go|panic|error|Error|\.go:\d+' } | Select-Object -First 40
            foreach ($l in $lines) { Write-Host "::error::TurboMON ($what): $($l.Trim())" }
        }
        throw "$what falló"
    }
}

Step "go vet" { go vet ./... }
Step "pruebas" { go test -short ./... }
Step "wails build" { wails build -skipbindings -trimpath -ldflags "-s -w" }
Copy-Item .\build\bin\TurboMON.exe .\TurboMON.exe -Force
Write-Host "Listo: $(Resolve-Path .\TurboMON.exe)"

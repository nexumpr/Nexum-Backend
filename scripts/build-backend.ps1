<#
.SYNOPSIS
  Compila el backend Go para Linux ARM64, calcula SHA-256 y genera un manifiesto.

.DESCRIPTION
  Windows 11 + PowerShell 7. Sin Docker ni WSL. No usa credenciales AWS y no
  publica nada: solo produce artefactos locales en dist/.

  Cross-compilation: el backend debe compilar con CGO deshabilitado. Si alguna
  dependencia exige CGO, el script falla y hay que resolverlo en la spec
  correspondiente (no se oculta).

.PARAMETER Version
  Version semantica o identificador del artefacto (por defecto: commit corto).

.PARAMETER OutputDir
  Directorio de salida. Por defecto: dist/

.PARAMETER Force
  Sobrescribe un artefacto existente. Sin este flag, el script se detiene.

.EXAMPLE
  pwsh -File scripts/build-backend.ps1 -Version v0.9.0
#>
[CmdletBinding()]
param(
    [string]$Version = "",
    [string]$OutputDir = "dist",
    [switch]$Force
)

$ErrorActionPreference = "Stop"
$raiz = Split-Path -Parent $PSScriptRoot
Push-Location $raiz
try {
    # --- Preflight -----------------------------------------------------------
    $goVersion = (& go version) 2>$null
    if ($LASTEXITCODE -ne 0) { throw "Go no esta disponible en PATH." }
    Write-Host "Go detectado: $goVersion"

    $modulo = (& go list -m) 2>$null
    if ($LASTEXITCODE -ne 0) { throw "No se pudo determinar el modulo Go." }
    Write-Host "Modulo: $modulo"

    if (-not $Version) {
        $Version = (& git rev-parse --short HEAD).Trim()
        if ($LASTEXITCODE -ne 0) { throw "Sin -Version y sin git disponible." }
    }
    if ($Version -notmatch '^[A-Za-z0-9._-]{1,40}$') {
        throw "Version invalida: '$Version'."
    }

    $salida = Join-Path $raiz $OutputDir
    $binario = Join-Path $salida "nexum-backend-linux-arm64"
    $manifiesto = Join-Path $salida "nexum-backend-linux-arm64.manifest.json"

    if ((Test-Path $binario) -and -not $Force) {
        throw "El artefacto '$binario' ya existe. Use -Force para sobrescribirlo (preflight)."
    }
    New-Item -ItemType Directory -Force -Path $salida | Out-Null

    # --- Pruebas antes de compilar ------------------------------------------
    Write-Host "Ejecutando go test ./... -count=1"
    & go test ./... -count=1
    if ($LASTEXITCODE -ne 0) { throw "go test fallo; no se compila un artefacto sin pruebas verdes." }

    # --- Cross-compile -------------------------------------------------------
    $commit = (& git rev-parse HEAD).Trim()
    $fecha = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    $ldflags = "-s -w -X main.version=$Version -X main.commit=$commit"

    # Se conservan los valores previos y se restauran siempre (tambien si falla).
    $previo = @{
        CGO_ENABLED = $env:CGO_ENABLED
        GOOS        = $env:GOOS
        GOARCH      = $env:GOARCH
    }
    $env:CGO_ENABLED = "0"
    $env:GOOS = "linux"
    $env:GOARCH = "arm64"
    Write-Host "Compilando GOOS=linux GOARCH=arm64 CGO_ENABLED=0"
    & go build -trimpath -ldflags $ldflags -o $binario ./cmd/server
    $codigo = $LASTEXITCODE
    foreach ($clave in $previo.Keys) {
        if ($null -eq $previo[$clave]) {
            Remove-Item "Env:$clave" -ErrorAction SilentlyContinue
        }
        else {
            Set-Item "Env:$clave" $previo[$clave]
        }
    }
    if ($codigo -ne 0) { throw "go build fallo para linux/arm64." }

    # --- Integridad ----------------------------------------------------------
    $sha = (Get-FileHash -Algorithm SHA256 -Path $binario).Hash.ToLowerInvariant()
    $tamano = (Get-Item $binario).Length

    $datos = [ordered]@{
        artefacto     = [System.IO.Path]::GetFileName($binario)
        version       = $Version
        commit        = $commit
        fecha_utc     = $fecha
        go_version    = ($goVersion -replace '^go version ', '')
        goos          = "linux"
        goarch        = "arm64"
        cgo           = "0"
        trimpath      = $true
        ldflags       = $ldflags
        tamano_bytes  = $tamano
        sha256        = $sha
        comando_build = "go build -trimpath -ldflags `"$ldflags`" -o $binario ./cmd/server"
    }
    $datos | ConvertTo-Json -Depth 4 | Set-Content -Encoding utf8 -Path $manifiesto

    # Verificacion de integridad inmediata.
    $recalculado = (Get-FileHash -Algorithm SHA256 -Path $binario).Hash.ToLowerInvariant()
    if ($recalculado -ne $sha) { throw "El SHA-256 no es estable; artefacto no confiable." }

    Write-Host ""
    Write-Host "Artefacto:   $binario"
    Write-Host "Manifiesto:  $manifiesto"
    Write-Host "SHA-256:     $sha"
    Write-Host "Tamano:      $tamano bytes"
    Write-Host "Version:     $Version (commit $commit)"
    Write-Host ""
    Write-Host "Para publicar (fase autorizada): aws s3 cp `"$binario`" `"s3://<bucket>/nexum-backend/<version>/nexum-backend-linux-arm64`""
    Write-Host "El valor sha256 del manifiesto es el que debe recibir artefacto_sha256 en Terraform."
}
finally {
    Pop-Location
}

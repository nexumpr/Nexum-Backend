param(
    [string]$Repo = 'C:\Proyectos\Nexum\Nexum-Backend'
)
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $Repo
Write-Host '=== NEXUM PREPARACION: SOLO LECTURA ==='
$branch = git branch --show-current
$head = git rev-parse HEAD
$stat = git status --short
Write-Host "Branch: $branch"
Write-Host "HEAD:   $head"
Write-Host 'Cambios locales:'
$stat | ForEach-Object { Write-Host $_ }
if ($branch -ne 'main') { Write-Warning 'Rama no es main: detener integración y reconciliar.' }
if ($head -ne '8bc2548339d6fdef9211c27c157bf75568fa052d') { Write-Warning 'HEAD difiere de la base del ZIP: NO sobrescribir; comparar antes de integrar.' }
if (-not (Test-Path -LiteralPath '.\opencode.json')) { throw 'Falta opencode.json local' }
$hash = (Get-FileHash -LiteralPath '.\opencode.json' -Algorithm SHA256).Hash
Write-Host "SHA256 opencode.json LOCAL: $hash"
$localConf = Get-Content -LiteralPath '.\opencode.json' -Raw | ConvertFrom-Json
$gitConfText = (git show HEAD:opencode.json) -join "`n"
$gitConf = $gitConfText | ConvertFrom-Json
Write-Host "Permisos local/HEAD: $($localConf.permissions.Count)/$($gitConf.permissions.Count)"
Write-Host "Policies local/HEAD: $($localConf.policies.Count)/$($gitConf.policies.Count)"
if (@($stat).Count -ne 1 -or -not ($stat -match 'opencode.json')) {
    Write-Warning 'Working tree distinto al esperado: NO integrar sin reconciliación.'
}
try { Write-Host "OpenCode version: $(opencode --version)" } catch { Write-Warning 'OpenCode no disponible o falló consulta de version' }
try { Write-Host "Go version: $(go version)" } catch { Write-Warning 'Go no disponible' }
try { Write-Host "Terraform version: $((terraform version -json | ConvertFrom-Json).terraform_version)" } catch { Write-Warning 'Terraform no disponible' }
Write-Host 'NO se hicieron mutaciones AWS, Git ni del archivo local.'

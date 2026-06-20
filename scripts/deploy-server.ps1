#requires -Version 5.1
<#
.SYNOPSIS
  Build the BRUV server binary and upgrade the installed BRUV-Server
  service on a remote (home-server) machine over SSH/Tailscale.

.DESCRIPTION
  Option A from the deploy discussion: a push deploy. Locally it builds
  the unified Wails binary (the same one the installer ships and the
  BRUV-Server Windows service runs), scp's it to the remote box, then -
  over SSH - stops the service, swaps the binary (keeping a .bak), and
  restarts. Windows can't overwrite a running .exe, hence stop->swap->start.

  The remote target is self-discovered: the script reads the service's
  own configured binary path (Win32_Service.PathName), so there's no
  hardcoded install location. On any failure during the swap it rolls
  back to the .bak and restarts.

  PREREQUISITES on the home server (one-time):
    - OpenSSH Server enabled and the SSH user is a local Administrator
      (the swap touches Program Files + the service):
        Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0
        Start-Service sshd; Set-Service sshd -StartupType Automatic
    - BRUV already installed once (the BRUV-Server service exists).
    - Reachable over Tailscale (or LAN) from this machine.

  Config you don't want to pass each time can live in env vars:
    BRUV_DEPLOY_HOST, BRUV_DEPLOY_USER.

.EXAMPLE
  ./scripts/deploy-server.ps1 -RemoteHost homeserver.tailnet.ts.net -SshUser harvey

.EXAMPLE
  # Redeploy the last-built binary without rebuilding:
  ./scripts/deploy-server.ps1 -RemoteHost homeserver.ts.net -SkipBuild
#>
[CmdletBinding()]
param(
  [string]$RemoteHost = $env:BRUV_DEPLOY_HOST,
  [string]$SshUser    = $(if ($env:BRUV_DEPLOY_USER) { $env:BRUV_DEPLOY_USER } else { $env:USERNAME }),
  [string]$ServiceName = 'BRUV-Server',
  # Path to the locally-built binary that gets pushed. Defaulted in the
  # body (not here) because $PSScriptRoot is empty in param defaults on
  # some hosts. Default matches wails.json outputfilename ("bruv-1.0").
  [string]$LocalExe,
  # URL hit after the deploy to confirm the new build is live (unauthed
  # /version endpoint). Override for TLS-fronted servers (https / :443).
  [string]$HealthUrl,
  [switch]$SkipBuild,   # reuse the existing build/bin binary
  [switch]$NoBackup,    # don't keep a .bak (not recommended)
  [switch]$Force        # skip the confirmation prompt
)

$ErrorActionPreference = 'Stop'
function Info($m) { Write-Host $m -ForegroundColor Cyan }
function Ok($m)   { Write-Host $m -ForegroundColor Green }
function Warn($m) { Write-Host $m -ForegroundColor Yellow }
function Die($m)  { Write-Host $m -ForegroundColor Red; exit 1 }

$scriptDir = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
$repoRoot = (Resolve-Path (Join-Path $scriptDir '..')).Path
if (-not $LocalExe) { $LocalExe = Join-Path $repoRoot 'build\bin\bruv-1.0.exe' }

if (-not $RemoteHost) {
  Die "No remote host. Pass -RemoteHost <tailscale-host>, or set `$env:BRUV_DEPLOY_HOST."
}
$Target = "$SshUser@$RemoteHost"
if (-not $HealthUrl) { $HealthUrl = "http://${RemoteHost}:9870/version" }

# --- 1. Build (unless skipped) -------------------------------------------
if ($SkipBuild) {
  if (-not (Test-Path $LocalExe)) { Die "Binary not found: $LocalExe (drop -SkipBuild to build it)." }
  Info "Skipping build; deploying existing $LocalExe"
} else {
  $sha = ''
  try { $sha = (git -C $repoRoot rev-parse --short HEAD).Trim() } catch { }
  $version   = if ($sha) { "dev-$sha" } else { "dev" }
  $buildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
  Info "Building unified server binary (version=$version)..."
  Push-Location $repoRoot
  try {
    & wails build -platform windows/amd64 -trimpath `
      -ldflags "-X main.AppVersion=$version -X main.BuildDate=$buildDate"
    if ($LASTEXITCODE -ne 0) { Die "wails build failed (exit $LASTEXITCODE)." }
  } finally { Pop-Location }
  if (-not (Test-Path $LocalExe)) { Die "Build succeeded but $LocalExe is missing - check wails.json outputfilename." }
}
$size = [math]::Round((Get-Item $LocalExe).Length / 1MB, 1)
Info "Artifact: $LocalExe ($size MB)"

# --- 2. Confirm -----------------------------------------------------------
if (-not $Force) {
  Warn "About to upgrade service '$ServiceName' on $Target (stop -> swap binary -> start)."
  $ans = Read-Host "Proceed? [y/N]"
  if ($ans -notmatch '^(y|yes)$') { Die "Aborted." }
}

# --- 3. Stage the binary on the remote (lands in the SSH user's home) -----
Info "Copying binary to $Target ..."
& scp $LocalExe "${Target}:bruv-deploy.exe"
if ($LASTEXITCODE -ne 0) { Die "scp failed (exit $LASTEXITCODE). Is OpenSSH reachable on $RemoteHost?" }

# --- 4. Remote swap (stop -> backup -> replace -> start, with rollback) ------
# Single-quoted here-string keeps every $ literal for the *remote* shell;
# two placeholders get substituted before it's base64-encoded for
# powershell -EncodedCommand (avoids all cross-shell quoting issues).
$remote = @'
$ErrorActionPreference = 'Stop'
$svc = '__SVC__'
$DoBackup = __BACKUP__
$staged = Join-Path $env:USERPROFILE 'bruv-deploy.exe'
if (-not (Test-Path $staged)) { throw "staged binary missing at $staged" }

$svcObj = Get-CimInstance Win32_Service -Filter "Name='$svc'" -ErrorAction Stop
if (-not $svcObj) { throw "service '$svc' is not installed on this machine" }
if ($svcObj.PathName.Trim() -notmatch '^\s*"?(.+?\.exe)"?') { throw "could not parse service path: $($svcObj.PathName)" }
$exe = $matches[1]
$bak = "$exe.bak"
Write-Output "remote target: $exe"

Write-Output "stopping $svc..."
Stop-Service -Name $svc -Force
(Get-Service $svc).WaitForStatus('Stopped', '00:00:45')
Get-Process -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $exe } | Stop-Process -Force -ErrorAction SilentlyContinue

try {
  if ($DoBackup) { Copy-Item -LiteralPath $exe -Destination $bak -Force; Write-Output "backed up -> $bak" }
  Copy-Item -LiteralPath $staged -Destination $exe -Force
  Write-Output "binary replaced"
  Start-Service -Name $svc
  (Get-Service $svc).WaitForStatus('Running', '00:00:45')
  Write-Output "service running"
}
catch {
  Write-Output "FAILED: $($_.Exception.Message)"
  if ($DoBackup -and (Test-Path $bak)) {
    Copy-Item -LiteralPath $bak -Destination $exe -Force
    try { Start-Service -Name $svc; (Get-Service $svc).WaitForStatus('Running', '00:00:30') } catch { }
    Write-Output "rolled back to previous binary"
  }
  throw
}
finally { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
Write-Output "OK"
'@
$remote = $remote.Replace('__SVC__', $ServiceName).Replace('__BACKUP__', $(if ($NoBackup) { '$false' } else { '$true' }))
$encoded = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($remote))

Info "Deploying on $RemoteHost ..."
& ssh $Target "powershell -NoProfile -NonInteractive -EncodedCommand $encoded"
if ($LASTEXITCODE -ne 0) { Die "Remote deploy failed (exit $LASTEXITCODE). The service was rolled back if a backup existed." }

# --- 5. Confirm the new build is live ------------------------------------
Info "Verifying via $HealthUrl ..."
$confirmed = $false
for ($i = 0; $i -lt 4; $i++) {
  Start-Sleep -Seconds 2
  try {
    $v = Invoke-RestMethod -Uri $HealthUrl -TimeoutSec 5
    Ok "Server is up: version=$($v.version) build_date=$($v.build_date)"
    $confirmed = $true
    break
  } catch { }
}
if (-not $confirmed) {
  Warn "Deployed + service started, but couldn't reach $HealthUrl to confirm."
  Warn "If your server is TLS-fronted, pass -HealthUrl https://$RemoteHost/version"
}
Ok "Done."

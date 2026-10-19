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
  The build also emits the NSIS installer (needs NSIS locally), which is
  kept on the server the same way the UNCLI sibling project keeps its
  builds: in a folder in the SSH user's home (bruv-builds), one file per
  deploy named bruv-setup-<yyyy-MM-dd>-<NNN>.exe, the number counting up
  across deploys, and only the newest -KeepBuilds (6) kept. So the server
  always holds an installer matching the service it runs, plus a few to
  roll back to. -SkipInstaller opts out.

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
  ./scripts/deploy-server.ps1 -RemoteHost homeserver.tailnet.ts.net -SshUser admin

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
  # Folder (in the SSH user's home) that keeps the installers, one per
  # deploy: bruv-setup-<date>-<number>.exe. BRUV_DEPLOY_BUILDS_DIR overrides.
  [string]$RemoteBuildsDir = $(if ($env:BRUV_DEPLOY_BUILDS_DIR) { $env:BRUV_DEPLOY_BUILDS_DIR } else { 'bruv-builds' }),
  # How many installers to keep there; older ones are deleted.
  [ValidateRange(1, 1000)][int]$KeepBuilds = 6,
  [switch]$SkipBuild,     # reuse the existing build/bin binary (and installer, if present)
  [switch]$SkipInstaller, # don't build or push the NSIS installer
  [switch]$NoBackup,      # don't keep a .bak (not recommended)
  [switch]$Force          # skip the confirmation prompt
)

$ErrorActionPreference = 'Stop'
function Info($m) { Write-Host $m -ForegroundColor Cyan }
function Ok($m)   { Write-Host $m -ForegroundColor Green }
function Warn($m) { Write-Host $m -ForegroundColor Yellow }
function Die($m)  { Write-Host $m -ForegroundColor Red; exit 1 }

$scriptDir = if ($PSScriptRoot) { $PSScriptRoot } else { Split-Path -Parent $MyInvocation.MyCommand.Path }
$repoRoot = (Resolve-Path (Join-Path $scriptDir '..')).Path
if (-not $LocalExe) { $LocalExe = Join-Path $repoRoot 'build\bin\bruv-1.0.exe' }
# The NSIS installer wails emits next to the binary. Pushed to the SSH
# user's home on the server after every deploy so the box always holds an
# installer matching the service it runs — a fresh laptop on the tailnet
# can fetch it without a GitHub round-trip.
$LocalInstaller = Join-Path (Split-Path -Parent $LocalExe) 'bruv-amd64-installer.exe'

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
  # The commit's own time, not the build's: clients compare it to spot a
  # server running older code (a stale checkout deploys a fresh build_date).
  $commitDate = ''
  try { $commitDate = (git -C $repoRoot log -1 --format=%cI).Trim() } catch { }
  # -nsis makes wails emit the installer alongside the binary. wails spawns
  # makensis as a native process, so its directory must be on the WINDOWS
  # PATH; a missing NSIS makes wails skip the installer silently (the
  # release workflow learned this the hard way), hence the explicit check.
  $buildArgs = @('build', '-platform', 'windows/amd64', '-trimpath',
    '-ldflags', "-X main.AppVersion=$version -X main.BuildDate=$buildDate -X main.CommitDate=$commitDate")
  if (-not $SkipInstaller) {
    $makensis = Get-Command makensis -ErrorAction SilentlyContinue
    if (-not $makensis) {
      $candidate = @("${env:ProgramFiles(x86)}\NSIS\makensis.exe", "$env:ProgramFiles\NSIS\makensis.exe") |
        Where-Object { Test-Path $_ } | Select-Object -First 1
      if ($candidate) { $env:PATH = (Split-Path -Parent $candidate) + ';' + $env:PATH; $makensis = $candidate }
    }
    if ($makensis) { $buildArgs += '-nsis' }
    else { Warn "NSIS (makensis) not found - building without the installer. Install NSIS or pass -SkipInstaller to silence this." }
  }
  Info "Building unified server binary (version=$version)$(if ($buildArgs -contains '-nsis') { ' + installer' })..."
  Push-Location $repoRoot
  try {
    & wails @buildArgs
    if ($LASTEXITCODE -ne 0) { Die "wails build failed (exit $LASTEXITCODE)." }
  } finally { Pop-Location }
  if (-not (Test-Path $LocalExe)) { Die "Build succeeded but $LocalExe is missing - check wails.json outputfilename." }
  if (($buildArgs -contains '-nsis') -and -not (Test-Path $LocalInstaller)) {
    Warn "wails built without emitting $LocalInstaller - the installer will not be updated on the server."
  }
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
& scp -o BatchMode=yes $LocalExe "${Target}:bruv-deploy.exe"
if ($LASTEXITCODE -ne 0) { Die "scp failed (exit $LASTEXITCODE). Is OpenSSH reachable on $RemoteHost?" }

# --- 3b. Keep the installer on the server, numbered --------------------
# Same scheme as the UNCLI sibling project's uncli-builds: one file per
# deploy, bruv-setup-<yyyy-MM-dd>-<NNN>.exe in $RemoteBuildsDir, NNN one
# higher than the highest already there, newest $KeepBuilds kept. The old
# fixed bruv-amd64-installer.exe in the home folder is retired the first
# time. Secondary to the service swap, so a failure warns, never aborts.
function Invoke-RemotePS([string]$script) {
  $enc = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($script))
  & ssh -o BatchMode=yes $Target "powershell -NoProfile -NonInteractive -EncodedCommand $enc"
}

$installerPushed = $null
if ($SkipInstaller) {
  Info "Skipping installer (-SkipInstaller)."
} elseif (-not (Test-Path $LocalInstaller)) {
  Warn "No installer at $LocalInstaller - the server's builds are left as-is."
} else {
  $nextNumber = @'
$dir = Join-Path $env:USERPROFILE '__DIR__'
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$max = 0
Get-ChildItem -LiteralPath $dir -Filter 'bruv-setup-*.exe' | ForEach-Object {
  if ($_.BaseName -match '-(\d+)$') { $n = [int]$matches[1]; if ($n -gt $max) { $max = $n } }
}
Write-Output ($max + 1)
'@
  $next = (Invoke-RemotePS $nextNumber.Replace('__DIR__', $RemoteBuildsDir) | Select-Object -Last 1)
  if ($LASTEXITCODE -ne 0 -or "$next" -notmatch '^\d+$') {
    Warn "Couldn't read the build number from ${Target}:$RemoteBuildsDir - installer not pushed."
  } else {
    $buildName = 'bruv-setup-{0}-{1:D3}.exe' -f (Get-Date -Format 'yyyy-MM-dd'), [int]$next
    $isize = [math]::Round((Get-Item $LocalInstaller).Length / 1MB, 1)
    Info "Copying installer ($isize MB) to ${Target}:$RemoteBuildsDir/$buildName ..."
    & scp -o BatchMode=yes $LocalInstaller "${Target}:$RemoteBuildsDir/$buildName"
    if ($LASTEXITCODE -ne 0) {
      Warn "Installer scp failed (exit $LASTEXITCODE) - continuing with the service swap."
    } else {
      $installerPushed = "$RemoteBuildsDir/$buildName"
      $prune = @'
$dir = Join-Path $env:USERPROFILE '__DIR__'
$builds = Get-ChildItem -LiteralPath $dir -Filter 'bruv-setup-*.exe' |
  Where-Object { $_.BaseName -match '-(\d+)$' } |
  Sort-Object { [int]($_.BaseName -replace '^.*-(\d+)$', '$1') } -Descending
$builds | Select-Object -Skip __KEEP__ | ForEach-Object { Remove-Item -LiteralPath $_.FullName -Force; Write-Output "removed old build $($_.Name)" }
$legacy = Join-Path $env:USERPROFILE 'bruv-amd64-installer.exe'
if (Test-Path -LiteralPath $legacy) { Remove-Item -LiteralPath $legacy -Force; Write-Output "retired $legacy (builds now live in $dir)" }
'@
      $pruned = @(Invoke-RemotePS $prune.Replace('__DIR__', $RemoteBuildsDir).Replace('__KEEP__', [string]$KeepBuilds))
      if ($LASTEXITCODE -ne 0) { Warn "Pruning old builds failed (exit $LASTEXITCODE) - they're left in place." }
      $pruned | ForEach-Object { Write-Host $_ }
      # One JSON line for the build log, the same shape UNCLI's publish-build prints.
      $buildRecord = [ordered]@{
        build   = [int]$next
        name    = $buildName
        date    = (Get-Date -Format 'yyyy-MM-dd')
        bytes   = (Get-Item $LocalInstaller).Length
        sha256  = (Get-FileHash -Algorithm SHA256 -LiteralPath $LocalInstaller).Hash.ToLower()
        path    = "$RemoteBuildsDir/$buildName"
        version = $version
        removed = @($pruned | Where-Object { $_ -match '^removed old build (.+)$' } | ForEach-Object { $matches[1] })
      }
    }
  }
}

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

# File handles can outlive SERVICE_STOPPED by a moment (process teardown,
# AV re-scan, a desktop instance closing). A locked target fails the copy
# BEFORE writing, so retrying is safe and the on-disk exe stays valid.
function Copy-WithRetry([string]$src, [string]$dst) {
  for ($i = 1; $i -le 6; $i++) {
    try { Copy-Item -LiteralPath $src -Destination $dst -Force; return }
    catch {
      if ($i -eq 6) { throw }
      Write-Output "copy locked (attempt $i/6) - retrying in 2s..."
      Start-Sleep -Seconds 2
    }
  }
}

try {
  if ($DoBackup) { Copy-WithRetry $exe $bak; Write-Output "backed up -> $bak" }
  Copy-WithRetry $staged $exe
  Write-Output "binary replaced"
  Start-Service -Name $svc
  (Get-Service $svc).WaitForStatus('Running', '00:00:45')
  Write-Output "service running"
}
catch {
  Write-Output "FAILED: $($_.Exception.Message)"
  if ($DoBackup -and (Test-Path $bak)) {
    try { Copy-WithRetry $bak $exe; Write-Output "rolled back to previous binary" }
    catch { Write-Output "rollback copy failed - on-disk binary left as-is (a locked copy never modifies the target)" }
  }
  # Never leave the box headless: whatever happened above, the on-disk
  # exe is a runnable binary, so always try to bring the service back.
  # (A 2026-07-11 deploy hit a transient lock and left the service
  # stopped until a manual restart - this is the fix.)
  try {
    Start-Service -Name $svc
    (Get-Service $svc).WaitForStatus('Running', '00:00:30')
    Write-Output "service restarted on previous binary"
  } catch {
    Write-Output "WARNING: service could not be restarted - start it manually: sc start $svc"
  }
  throw
}
finally { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
Write-Output "OK"
'@
$remote = $remote.Replace('__SVC__', $ServiceName).Replace('__BACKUP__', $(if ($NoBackup) { '$false' } else { '$true' }))
$encoded = [Convert]::ToBase64String([System.Text.Encoding]::Unicode.GetBytes($remote))

Info "Deploying on $RemoteHost ..."
& ssh -o BatchMode=yes $Target "powershell -NoProfile -NonInteractive -EncodedCommand $encoded"
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
if ($installerPushed) {
  Ok "Installer kept at ${Target}:$installerPushed (newest $KeepBuilds kept)"
  if ($buildRecord) { Write-Output ($buildRecord | ConvertTo-Json -Compress) }
}
Ok "Done."

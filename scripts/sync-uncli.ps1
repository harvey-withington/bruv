#requires -Version 5.1
<#
.SYNOPSIS
  Bring a new UNCLI build into BRUV: rebuild UNCLI's chat packages and
  reinstall them where BRUV uses them.

.DESCRIPTION
  BRUV embeds UNCLI (its chat engine) from a local UNCLI checkout until
  UNCLI is published: the Go module through go.mod's relative replace,
  and the chat element as two npm tarballs (@uncli/client, @uncli/chat)
  that UNCLI packs into app/frontend/dist-embed/packs. A rebuilt tarball
  keeps its version and path, so npm won't pick it up by itself; this
  removes node_modules/@uncli and reinstalls it in frontend/, mobile/
  and the root (whose shared/ code imports the element's types).

  Afterwards it checks the desktop's vendored dotlottie-web asset is
  still there (a nested reinstall once lost it and broke the build) and
  reinstalls frontend/'s packages if not.

  The Go side needs nothing: the replace points at the checkout, so the
  next build uses UNCLI's Go code as it is.

.PARAMETER UncliRoot
  The UNCLI checkout. Default: $env:BRUV_UNCLI_DIR, else ../../UNCLI/UNCLI-1.0
  from the repo root (the layout go.mod's replace expects).

.PARAMETER SkipPack
  Reinstall the tarballs already in dist-embed/packs without rebuilding them.

.EXAMPLE
  ./scripts/sync-uncli.ps1

.EXAMPLE
  ./scripts/sync-uncli.ps1 -SkipPack
#>
[CmdletBinding()]
param(
  [string]$UncliRoot = $env:BRUV_UNCLI_DIR,
  [switch]$SkipPack
)

$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
if (-not $UncliRoot) { $UncliRoot = Join-Path $repo '..\..\UNCLI\UNCLI-1.0' }
$UncliRoot = (Resolve-Path $UncliRoot).Path
$packs = Join-Path $UncliRoot 'app\frontend\dist-embed\packs'

function Step([string]$text) { Write-Host "==> $text" -ForegroundColor Cyan }

# Runs npm, judged by its exit code alone: Windows PowerShell turns npm's
# stderr (deprecation warnings) into errors, which 'Stop' would make fatal.
function Invoke-Npm([string]$what, [string[]]$npmArgs) {
  $ErrorActionPreference = 'Continue'
  $output = & npm @npmArgs 2>&1
  if ($LASTEXITCODE -ne 0) {
    $output | ForEach-Object { Write-Host $_ }
    throw "$what failed (exit $LASTEXITCODE)"
  }
}

if (-not $SkipPack) {
  Step "Packing UNCLI's chat packages in $UncliRoot"
  Invoke-Npm 'pack:packages' @('--prefix', (Join-Path $UncliRoot 'app\frontend'), 'run', 'pack:packages')
}
$tarballs = @(Get-ChildItem $packs -Filter 'uncli-*.tgz')
if ($tarballs.Count -ne 2) { throw "Expected the client and chat tarballs in $packs, found: $($tarballs.Name -join ', ')" }
$tarballs | ForEach-Object { Write-Host ("    {0}  {1:yyyy-MM-dd HH:mm}" -f $_.Name, $_.LastWriteTime) }

# A path from dir to a tarball, relative and with forward slashes, as
# package.json already records it (so reinstalling doesn't rewrite it).
function Relative([string]$dir, [string]$file) {
  $from = New-Object System.Uri ((Resolve-Path $dir).Path.TrimEnd('\') + '\')
  [System.Uri]::UnescapeDataString($from.MakeRelativeUri((New-Object System.Uri $file)).ToString())
}

# The root goes last: it links bruv-frontend (file:frontend), so its install
# also resolves frontend's package.json, which must already name the new
# tarballs after a version bump.
foreach ($target in @(
    @{ Dir = (Join-Path $repo 'frontend'); Args = @() },
    @{ Dir = (Join-Path $repo 'mobile'); Args = @() },
    @{ Dir = $repo; Args = @('--ignore-scripts') })) {  # root: its postinstall reinstalls both surfaces
  Step "Reinstalling in $(Split-Path -Leaf $target.Dir)"
  Push-Location $target.Dir
  try {
    Remove-Item -Recurse -Force 'node_modules\@uncli' -ErrorAction SilentlyContinue
    $paths = $tarballs | ForEach-Object { Relative $target.Dir $_.FullName }
    Invoke-Npm "npm install in $($target.Dir)" (@('install') + $target.Args + $paths)
  } finally {
    Pop-Location
  }
}

$wasm = Join-Path $repo 'frontend\node_modules\@lottiefiles\dotlottie-web\dist\dotlottie-player.wasm'
if (-not (Test-Path $wasm)) {
  Step 'The desktop animation package went missing; reinstalling frontend/'
  Invoke-Npm 'reinstalling frontend' @('install', '--prefix', (Join-Path $repo 'frontend'))
  if (-not (Test-Path $wasm)) { throw "Still missing after reinstall: $wasm" }
}

Step 'Done. Run npm run check, then build or deploy as usual.'

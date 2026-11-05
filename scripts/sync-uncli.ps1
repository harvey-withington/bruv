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
  removes node_modules/@uncli and reinstalls it in the root (whose
  shared/ code imports the element's types), frontend/ and mobile/.

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

if (-not $SkipPack) {
  Step "Packing UNCLI's chat packages in $UncliRoot"
  npm --prefix (Join-Path $UncliRoot 'app\frontend') run pack:packages | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "pack:packages failed (exit $LASTEXITCODE)" }
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

foreach ($target in @(
    @{ Dir = $repo; Args = @('--ignore-scripts') },  # root: its postinstall reinstalls both surfaces
    @{ Dir = (Join-Path $repo 'frontend'); Args = @() },
    @{ Dir = (Join-Path $repo 'mobile'); Args = @() })) {
  Step "Reinstalling in $(Split-Path -Leaf $target.Dir)"
  Push-Location $target.Dir
  try {
    Remove-Item -Recurse -Force 'node_modules\@uncli' -ErrorAction SilentlyContinue
    $paths = $tarballs | ForEach-Object { Relative $target.Dir $_.FullName }
    npm install @($target.Args) @paths | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "npm install failed in $($target.Dir) (exit $LASTEXITCODE)" }
  } finally {
    Pop-Location
  }
}

$wasm = Join-Path $repo 'frontend\node_modules\@lottiefiles\dotlottie-web\dist\dotlottie-player.wasm'
if (-not (Test-Path $wasm)) {
  Step 'The desktop animation package went missing; reinstalling frontend/'
  npm install --prefix (Join-Path $repo 'frontend') | Out-Null
  if (-not (Test-Path $wasm)) { throw "Still missing after reinstall: $wasm" }
}

Step 'Done. Run npm run check, then build or deploy as usual.'

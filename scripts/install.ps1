# yuque-cli installer for Windows (PowerShell 5.1+)
#
# NOTE: keep this file pure ASCII. Windows PowerShell 5.1 reads a BOM-less file as ANSI,
#       and a BOM breaks `irm | iex`, so non-ASCII text can fail to parse. CI enforces this.
#
# Usage:
#   irm https://raw.githubusercontent.com/ipfred/yuque-cli/main/scripts/install.ps1 | iex
#
# Environment variables:
#   YUQUE_CLI_VERSION        version to install, e.g. 0.2.0 or v0.2.0 (default: latest GitHub Release)
#   YUQUE_CLI_BIN_DIR        install directory (default: %LOCALAPPDATA%\yuque-cli)
#   YUQUE_CLI_DOWNLOAD_BASE  download URL prefix (default: GitHub Releases; override for a mirror,
#                            files are fetched from $BASE/v<version>/)
#   YUQUE_CLI_SKIP_VERIFY    set to 1 to skip SHA256 verification (only for mirrors without SHA256SUMS)

# Everything runs in a child scope. Under `irm | iex` the code executes in the caller's scope, so
# we must not call `exit` (it would close the user's PowerShell) and must not leak
# $ErrorActionPreference.
& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'   # the progress bar slows Invoke-WebRequest down a lot in PS 5.1

  $Repo = 'ipfred/yuque-cli'

  # PS 5.1 may not enable TLS 1.2 by default.
  try { [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12 } catch {}

  function Say($msg) { Write-Host $msg }

  # Download with timeout and retries.
  function Get-File($uri, $outFile) {
    for ($i = 1; $i -le 3; $i++) {
      try {
        Invoke-WebRequest -Uri $uri -OutFile $outFile -UseBasicParsing -TimeoutSec 120
        return
      } catch {
        if ($i -eq 3) { throw }
        Start-Sleep -Seconds 2
      }
    }
  }

  # Latest tag: prefer the /releases/latest redirect (not subject to the anonymous API rate limit),
  # fall back to the API.
  function Get-LatestTag {
    try {
      $r = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -UseBasicParsing -TimeoutSec 30
      $final = if ($r.BaseResponse.ResponseUri) { $r.BaseResponse.ResponseUri.AbsoluteUri }      # Windows PowerShell 5.1
               else { $r.BaseResponse.RequestMessage.RequestUri.AbsoluteUri }                      # PowerShell 7+
      $tag = ($final -split '/')[-1]
      if ($tag -match '^v\d') { return $tag }
    } catch {}
    $rel = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing -TimeoutSec 30
    return $rel.tag_name
  }

  $tmp = $null
  try {
    # ---------- platform ----------
    # A 32-bit PowerShell on 64-bit Windows reports x86 in PROCESSOR_ARCHITECTURE;
    # the real architecture is in PROCESSOR_ARCHITEW6432.
    $rawArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    $arch = switch ($rawArch) {
      'AMD64' { 'amd64' }
      'ARM64' { 'arm64' }
      default { throw "Unsupported CPU architecture: $rawArch (windows amd64 and arm64 are provided)" }
    }

    # ---------- version ----------
    $version = $env:YUQUE_CLI_VERSION
    if (-not $version) {
      Say '>> Looking up the latest version...'
      try { $version = Get-LatestTag }
      catch { throw "Could not determine the latest version. Set `$env:YUQUE_CLI_VERSION = '<version>' to skip the lookup." }
    }
    $version = $version -replace '^v', ''
    # The version ends up in a URL and a file name: letters, digits, dot, underscore, hyphen only.
    if ($version -notmatch '^[0-9A-Za-z][0-9A-Za-z._-]*$') { throw "Invalid version: $version" }

    $asset = "yuque-cli_${version}_windows_${arch}.zip"
    $binDir = if ($env:YUQUE_CLI_BIN_DIR) { $env:YUQUE_CLI_BIN_DIR } else { Join-Path $env:LOCALAPPDATA 'yuque-cli' }
    $base   = if ($env:YUQUE_CLI_DOWNLOAD_BASE) { $env:YUQUE_CLI_DOWNLOAD_BASE } else { "https://github.com/$Repo/releases/download" }
    $url    = "$base/v$version/$asset"

    # ---------- download + SHA256 ----------
    $tmp = Join-Path ([IO.Path]::GetTempPath()) "yuque-cli-install-$(Get-Random)"
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    $zip = Join-Path $tmp $asset

    Say ">> Downloading $url"
    try { Get-File $url $zip }
    catch { throw "Download failed. Version $version may not exist, may have no windows/$arch package, or the network is unreachable." }

    if ($env:YUQUE_CLI_SKIP_VERIFY -eq '1') {
      Say '>> WARNING: SHA256 verification skipped (YUQUE_CLI_SKIP_VERIFY=1)'
    } else {
      $sums = Join-Path $tmp 'SHA256SUMS'
      try { Get-File "$base/v$version/SHA256SUMS" $sums }
      catch { throw 'Could not download SHA256SUMS; refusing to install an unverified file (set YUQUE_CLI_SKIP_VERIFY=1 for mirrors without it).' }

      # Compare outside the try/catch above so a mismatch is never mistaken for a download problem.
      $want = $null
      foreach ($line in Get-Content $sums) {
        $parts = $line.Trim() -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1].TrimStart('*') -eq $asset) { $want = $parts[0]; break }
      }
      if (-not $want) { throw "SHA256SUMS has no entry for $asset; refusing to install." }
      $got = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash
      if ($got -ne $want) { throw 'SHA256 verification failed.' }
      Say '>> SHA256 verified'
    }

    # ---------- extract + install ----------
    Expand-Archive -Path $zip -DestinationPath $tmp -Force
    $exe = Get-ChildItem -Path $tmp -Recurse -Filter 'yuque-cli.exe' | Select-Object -First 1
    if (-not $exe) { throw 'yuque-cli.exe not found in the archive.' }

    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
    $dst = Join-Path $binDir 'yuque-cli.exe'
    $old = "$dst.old"
    # A running exe cannot be overwritten but can be renamed: move the old one aside first.
    Remove-Item $old -Force -ErrorAction SilentlyContinue
    if (Test-Path $dst) {
      try { Move-Item $dst $old -Force }
      catch { throw "Cannot replace $dst. Close any running yuque-cli and try again." }
    }
    Copy-Item $exe.FullName $dst -Force
    Remove-Item $old -Force -ErrorAction SilentlyContinue
    Say ">> Installed to $dst"

    # ---------- user PATH (persisted in the registry) ----------
    # Use the registry directly instead of [Environment]::SetEnvironmentVariable so the existing
    # REG_EXPAND_SZ type and any %VAR% references in Path are preserved.
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
      $userPath = [string]$key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
      $kind = if ($key.GetValueNames() -contains 'Path') { $key.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
      $entries = @($userPath -split ';' | Where-Object { $_ -ne '' })   # @() keeps a single entry from being treated as a string
      if (($entries | ForEach-Object { $_.TrimEnd('\') }) -notcontains $binDir.TrimEnd('\')) {
        $key.SetValue('Path', (($entries + $binDir) -join ';'), $kind)
        # Writing the registry does not notify running programs. Adding and removing a throwaway
        # variable makes .NET broadcast WM_SETTINGCHANGE so newly opened terminals see the new Path.
        [Environment]::SetEnvironmentVariable('YUQUE_CLI_PATH_REFRESH', '1', 'User')
        [Environment]::SetEnvironmentVariable('YUQUE_CLI_PATH_REFRESH', $null, 'User')
        Say ">> Added $binDir to the user PATH"
      }
    } finally {
      $key.Close()
    }
    if (($env:Path -split ';' | ForEach-Object { $_.TrimEnd('\') }) -notcontains $binDir.TrimEnd('\')) {
      $env:Path = "$env:Path;$binDir"
      Say '>> Available in this session now; terminals that are already open must be reopened.'
    }

    # ---------- verify ----------
    Say ''
    Say '>> Verify:'
    & $dst version

    Say ''
    Say 'Installed. Next steps:'
    Say '  yuque-cli skill install                          # install the yuque-reader skill into your AI clients'
    Say '  yuque-cli auth login <any Yuque document URL>    # log in to Yuque'
  } catch {
    Write-Host "ERROR: $($_.Exception.Message)" -ForegroundColor Red
    $global:LASTEXITCODE = 1
    # Run as a file (CI, debugging) -> non-zero exit code. Under iex $PSCommandPath is empty; never exit there.
    if ($PSCommandPath) { exit 1 }
  } finally {
    if ($tmp) { Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue }
  }
}

const { execFile } = require('child_process');
const fs = require('fs');
const path = require('path');
const os = require('os');

const TOAST_HELPER = `
function Show-BruvToast($Title, $Message) {
  [Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
  [Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom, ContentType = WindowsRuntime] | Out-Null

  $template = @"
<toast scenario="reminder">
  <visual>
    <binding template="ToastGeneric">
      <text>$Title</text>
      <text>$Message</text>
    </binding>
  </visual>
</toast>
"@

  $xml = New-Object Windows.Data.Xml.Dom.XmlDocument
  $xml.LoadXml($template)
  $toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
  $appId = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\\WindowsPowerShell\\v1.0\\powershell.exe'
  [Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($appId).Show($toast)
}
`;

function activate(_context) {
  // Show open reminder as a Windows balloon/toast notification
  const openScript = path.join(os.tmpdir(), 'bruv-odrive-open.ps1');
  fs.writeFileSync(openScript, [
    TOAST_HELPER,
    `Show-BruvToast 'BRUV - oDrive Reminder' 'Remember to pause oDrive sync while working!'`,
    `Remove-Item -LiteralPath '${openScript.replace(/\\/g, '\\\\')}' -ErrorAction SilentlyContinue`
  ].join('\n'));

  execFile('powershell', [
    '-NoProfile', '-NonInteractive', '-WindowStyle', 'Hidden',
    '-ExecutionPolicy', 'Bypass', '-File', openScript
  ], { windowsHide: true });

  // Spawn a watcher that shows a close reminder after the editor exits.
  // We use WMI (Win32_Process.Create) with Win32_ProcessStartup (ShowWindow=0)
  // to escape the editor's Job Object and stay fully hidden.
  const ppid = process.ppid;
  const closeScript = path.join(os.tmpdir(), 'bruv-odrive-close.ps1');
  fs.writeFileSync(closeScript, [
    TOAST_HELPER,
    `try { Wait-Process -Id ${ppid} -ErrorAction Stop } catch {}`,
    `Show-BruvToast 'BRUV - oDrive Reminder' 'Remember to resume oDrive sync!'`,
    `Remove-Item -LiteralPath '${closeScript.replace(/\\/g, '\\\\')}' -ErrorAction SilentlyContinue`
  ].join('\n'));

  const wmiCmd = [
    `$si = ([wmiclass]'Win32_ProcessStartup').CreateInstance()`,
    `$si.ShowWindow = 0`,
    `([wmiclass]'Win32_Process').Create('powershell -NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -File "${closeScript}"', $null, $si)`
  ].join('; ');

  execFile('powershell', [
    '-NoProfile', '-NonInteractive', '-WindowStyle', 'Hidden',
    '-Command', wmiCmd
  ], { windowsHide: true });
}

function deactivate() {}

module.exports = { activate, deactivate };

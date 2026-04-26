Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them with the values from ProjectInfo.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
##
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
####
!include "wails_tools.nsh"

# The version information for this two must consist of 4 parts
VIProductVersion "${INFO_PRODUCTVERSION}.0"
VIFileVersion    "${INFO_PRODUCTVERSION}.0"

VIAddVersionKey "CompanyName"     "${INFO_COMPANYNAME}"
VIAddVersionKey "FileDescription" "${INFO_PRODUCTNAME} Installer"
VIAddVersionKey "ProductVersion"  "${INFO_PRODUCTVERSION}"
VIAddVersionKey "FileVersion"     "${INFO_PRODUCTVERSION}"
VIAddVersionKey "LegalCopyright"  "${INFO_COPYRIGHT}"
VIAddVersionKey "ProductName"     "${INFO_PRODUCTNAME}"

# Enable HiDPI support. https://nsis.sourceforge.io/Reference/ManifestDPIAware
ManifestDPIAware true

!include "MUI.nsh"
!include "LogicLib.nsh"

!define MUI_ICON "..\icon.ico"
!define MUI_UNICON "..\icon.ico"
!define MUI_FINISHPAGE_NOAUTOCLOSE
!define MUI_ABORTWARNING

# --- Pages ---
#
# Component selection lets the operator pick desktop-only,
# server-only, or both. The desktop checkbox is default-on; the
# server checkbox is opt-in (most users won't want a background
# service running on their machine).

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

# Server-mode installs surface their bootstrap-token / URL via a
# MessageBox at the end of SecServer (see below). The finish page
# itself stays default — trying to drive it from a runtime variable
# is fragile because MUI_FINISHPAGE_TEXT is parsed at compile time,
# not when the page is shown.
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

## Sign hooks (uncomment when SignPath / signtool wiring is ready)
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe"
InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
ShowInstDetails show

Var ServerRepoPath

Function .onInit
   !insertmacro wails.checkArchitecture
   # Default repo path for server installs. The user can change this
   # later by uninstalling + reinstalling the service with a different
   # --repo flag.
   StrCpy $ServerRepoPath "$APPDATA\${INFO_PRODUCTNAME}\server-repo"
FunctionEnd

# Component descriptions shown beneath the checkboxes on the components page.
LangString DESC_SecDesktop ${LANG_ENGLISH} "The BRUV desktop app. Installs the program, Start Menu shortcut, and file associations."
LangString DESC_SecServer ${LANG_ENGLISH} "Run BRUV as a background Windows service so this machine acts as a hosted backend other devices can connect to (over Tailscale, typically). Auto-starts on boot."

# --- Sections ---
#
# Both sections write the same binaries into $INSTDIR — the desktop
# and the server are the same bruv.exe in different invocation
# modes. Splitting them as components is purely about post-install
# steps (shortcuts, service registration, finish-page hints).

Section "Desktop App" SecDesktop
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    SetOutPath $INSTDIR

    !insertmacro wails.files

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols

    !insertmacro wails.writeUninstaller
SectionEnd

Section /o "Server (run in background, auto-start on boot)" SecServer
    # If the desktop section wasn't selected we still need the binary.
    # Wails' wails.files macro is idempotent (overwrites in place), so
    # invoking it again is safe and ensures bruv.exe lives in $INSTDIR
    # before we register the service.
    !insertmacro wails.setShellContext
    SetOutPath $INSTDIR
    !insertmacro wails.files
    !insertmacro wails.writeUninstaller

    # Register the Windows Service. bruv.exe service install creates
    # the repo at $ServerRepoPath if it doesn't exist, registers the
    # service to auto-start, and starts it. Output goes into the
    # install log (visible because ShowInstDetails show).
    # Punch a Windows Firewall hole for inbound TCP 9870 so other
    # tailnet devices can actually reach the server. Without this the
    # service runs fine, the port binds, but Windows Firewall silently
    # drops every inbound connection from off-machine — clients get
    # "Failed to fetch" with no log line on either end. The rule is
    # idempotent at the netsh level: if it already exists it'll error
    # but we don't care, the rule is in place either way.
    DetailPrint "Adding Windows Firewall rule for TCP 9870..."
    nsExec::ExecToLog 'netsh advfirewall firewall add rule name="BRUV Server" dir=in action=allow protocol=TCP localport=9870'
    Pop $0

    DetailPrint "Registering BRUV Server (repo: $ServerRepoPath)..."
    nsExec::ExecToLog '"$INSTDIR\${PRODUCT_EXECUTABLE}" service install --repo "$ServerRepoPath"'
    Pop $0
    ${If} $0 != "0"
        MessageBox MB_OK|MB_ICONEXCLAMATION "BRUV Server install returned $0. The desktop app is installed; you can register the server later from a terminal with: bruv.exe service install --repo <path>"
    ${Else}
        # Surface the bootstrap-token + URL info now, before the
        # generic finish page. Modal so the operator can't miss it.
        # Token lives in %PROGRAMDATA%\BRUV (machine-wide) rather
        # than %APPDATA% because the service runs as LocalSystem —
        # %APPDATA% in that context is C:\Windows\System32\... and
        # the user can't see it.
        MessageBox MB_OK|MB_ICONINFORMATION "BRUV Server is installed and running.$\r$\n$\r$\nRepo:                 $ServerRepoPath$\r$\nServer URL (local):   http://127.0.0.1:9870$\r$\nServer URL (Tailscale): http://<this-machine's tailnet IP>:9870$\r$\n$\r$\nThe one-time connection token for other devices is in:$\r$\n  %PROGRAMDATA%\BRUV\bootstrap-token.txt"
    ${EndIf}
SectionEnd

# Tooltip text on the components page.
!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
    !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} $(DESC_SecDesktop)
    !insertmacro MUI_DESCRIPTION_TEXT ${SecServer} $(DESC_SecServer)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

# --- Uninstall ---

Section "uninstall"
    !insertmacro wails.setShellContext

    # Best-effort: stop + uninstall the BRUV Server if it was registered.
    # Errors are silenced because the user might have only installed
    # the desktop component, in which case the service was never
    # registered and the call would just no-op.
    nsExec::ExecToLog '"$INSTDIR\${PRODUCT_EXECUTABLE}" service uninstall'
    Pop $0

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd

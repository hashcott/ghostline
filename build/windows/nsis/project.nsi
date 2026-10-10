Unicode true

####
## Please note: Template replacements don't work in this file. They are provided with default defines like
## mentioned underneath.
## If the keyword is not defined, "wails_tools.nsh" will populate them.
## If they are defined here, "wails_tools.nsh" will not touch them. This allows you to use this project.nsi manually
## from outside of Wails for debugging and development of the installer.
## 
## For development first make a wails nsis build to populate the "wails_tools.nsh":
## > wails build --target windows/amd64 --nsis
## Then you can call makensis on this file with specifying the path to your binary:
## For a AMD64 only installer:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app.exe
## For a ARM64 only installer:
## > makensis -DARG_WAILS_ARM64_BINARY=..\..\bin\app.exe
## For a installer with both architectures:
## > makensis -DARG_WAILS_AMD64_BINARY=..\..\bin\app-amd64.exe -DARG_WAILS_ARM64_BINARY=..\..\bin\app-arm64.exe
####
## The following information is taken from the wails_tools.nsh file, but they can be overwritten here.
####
## !define INFO_PROJECTNAME    "my-project" # Default "ghostline"
## !define INFO_COMPANYNAME    "My Company" # Default "Ghostline"
## !define INFO_PRODUCTNAME    "My Product Name" # Default "Ghostline"
## !define INFO_PRODUCTVERSION "1.0.0"     # Default "0.1.0"
## !define INFO_COPYRIGHT      "(c) Now, My Company" # Default "© 2026, My Company"
###
## !define PRODUCT_EXECUTABLE  "Application.exe"      # Default "${INFO_PROJECTNAME}.exe"
## !define UNINST_KEY_NAME     "UninstKeyInRegistry"  # Default "${INFO_COMPANYNAME}${INFO_PRODUCTNAME}"
####
## !define REQUEST_EXECUTION_LEVEL "admin"            # Default "admin"  see also https://nsis.sourceforge.io/Docs/Chapter4.html
## !define WAILS_INSTALL_SCOPE     "user"             # Default "machine" - set to "user" for per-user install ($LOCALAPPDATA) without UAC prompt
####
## Include the wails tools
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
# !define MUI_WELCOMEFINISHPAGE_BITMAP "resources\leftimage.bmp" #Include this to add a bitmap on the left side of the Welcome Page. Must be a size of 164x314
!define MUI_FINISHPAGE_NOAUTOCLOSE # Wait on the INSTFILES page so the user can take a look into the details of the installation steps
!define MUI_ABORTWARNING # This will warn the user if they exit from the installer.

!insertmacro MUI_PAGE_WELCOME # Welcome to the installer page.
# !insertmacro MUI_PAGE_LICENSE "resources\eula.txt" # Adds a EULA page to the installer
!insertmacro MUI_PAGE_DIRECTORY # In which folder install page.
!insertmacro MUI_PAGE_INSTFILES # Installing page.
!insertmacro MUI_PAGE_FINISH # Finished installation page.

!insertmacro MUI_UNPAGE_INSTFILES # Uninstalling page

!insertmacro MUI_LANGUAGE "English" # Set the Language of the installer

## The following two statements can be used to sign the installer and the uninstaller. The path to the binaries are provided in %1
#!uninstfinalize 'signtool --file "%1"'
#!finalize 'signtool --file "%1"'

Name "${INFO_PRODUCTNAME}"
OutFile "..\..\..\bin\${INFO_PROJECTNAME}-${ARCH}-installer.exe" # Name of the installer's file.
!if "${WAILS_INSTALL_SCOPE}" == "user"
    InstallDir "$LOCALAPPDATA\Programs\${INFO_PRODUCTNAME}"
!else
    InstallDir "$PROGRAMFILES64\${INFO_COMPANYNAME}\${INFO_PRODUCTNAME}"
!endif
ShowInstDetails show # This will always show the installation details.

; "/S /UPDATE": started by Ghostline's "install update" (spec: one-click
; update). The installer waits for Ghostline to quit by itself, so it can
; disconnect cleanly, and opens the new version at the end.
Var UpdateMode

Function .onInit
   !insertmacro wails.checkArchitecture
   StrCpy $UpdateMode "0"
   ${GetParameters} $R0
   ClearErrors
   ${GetOptions} $R0 "/UPDATE" $R1
   ${IfNot} ${Errors}
       StrCpy $UpdateMode "1"
   ${EndIf}
FunctionEnd

Section
    !insertmacro wails.setShellContext

    !insertmacro wails.webview2runtime

    ${If} $UpdateMode == "1"
        ; Up to 20 s for Ghostline (and its watchdog) to exit after the
        ; clean disconnect; whatever is left is killed below.
        StrCpy $R2 0
        ${Do}
            nsExec::ExecToStack 'cmd /c tasklist /FI "IMAGENAME eq ${PRODUCT_EXECUTABLE}" /NH | find /I "${PRODUCT_EXECUTABLE}"'
            Pop $R3
            Pop $R4
            ${If} $R3 != "0"
            ${OrIf} $R2 >= 40
                ${Break}
            ${EndIf}
            Sleep 500
            IntOp $R2 $R2 + 1
        ${Loop}
    ${EndIf}

    ; Stop a running Ghostline so its files can be replaced. A killed
    ; instance leaves state.json dirty; the next start restores DNS.
    nsExec::Exec 'taskkill /IM ${PRODUCT_EXECUTABLE} /F'

    SetOutPath $INSTDIR
    
    !insertmacro wails.files

    ; The taskkill above also killed the watchdog: restore DNS now if the
    ; killed instance was connected (no-op when state.json is clean).
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --restore'

    CreateShortcut "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"
    CreateShortCut "$DESKTOP\${INFO_PRODUCTNAME}.lnk" "$INSTDIR\${PRODUCT_EXECUTABLE}"

    !insertmacro wails.associateFiles
    !insertmacro wails.associateCustomProtocols
    
    !insertmacro wails.writeUninstaller

    ; Back to the user after an update; the app connects again if it was
    ; connected before (meta.json reconnectAfterUpdate).
    ${If} $UpdateMode == "1"
        Exec '"$INSTDIR\${PRODUCT_EXECUTABLE}"'
    ${EndIf}
SectionEnd

Section "uninstall" 
    !insertmacro wails.setShellContext

    ; Ghostline cleanup, in order: stop the app, restore DNS from its
    ; snapshot, remove the logon tasks, remove the WinDivert driver service.
    nsExec::Exec 'taskkill /IM ${PRODUCT_EXECUTABLE} /F'
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --restore'
    ; Remove every Ghostline root certificate (Fake SNI session CAs and
    ; the LAN CA) and the LAN CA files.
    ExecWait '"$INSTDIR\${PRODUCT_EXECUTABLE}" --remove-certs'
    nsExec::Exec 'netsh advfirewall firewall delete rule name="Ghostline Proxy"'
    nsExec::Exec 'netsh advfirewall firewall delete rule name="Ghostline DNS (TCP)"'
    nsExec::Exec 'netsh advfirewall firewall delete rule name="Ghostline DNS (UDP)"'
    nsExec::Exec 'netsh advfirewall firewall delete rule name="Ghostline Setup"'
    nsExec::Exec 'netsh advfirewall firewall delete rule name="Ghostline Block Public"'
    nsExec::Exec 'schtasks /Delete /TN "Ghostline" /F'
    nsExec::Exec 'schtasks /Delete /TN "Ghostline Recovery" /F'
    nsExec::Exec 'schtasks /Delete /TN "Ghostline Network Guard" /F'
    nsExec::Exec 'sc stop WinDivert'
    nsExec::Exec 'sc delete WinDivert'
    nsExec::Exec 'sc stop WinDivert1.4'
    nsExec::Exec 'sc delete WinDivert1.4'

    RMDir /r "$AppData\${PRODUCT_EXECUTABLE}" # Remove the WebView2 DataPath

    RMDir /r $INSTDIR

    Delete "$SMPROGRAMS\${INFO_PRODUCTNAME}.lnk"
    Delete "$DESKTOP\${INFO_PRODUCTNAME}.lnk"

    !insertmacro wails.unassociateFiles
    !insertmacro wails.unassociateCustomProtocols

    !insertmacro wails.deleteUninstaller
SectionEnd

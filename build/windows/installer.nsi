; SupportOne installer.
;
; Deliberately a per-user install: it writes only under the current user's own
; AppData and needs no administrator rights, so Windows raises no UAC prompt.
; That matches the program's own rule — it does not require admin to start, and
; asks for elevation only for the one action that needs it, at the moment it
; needs it. An installer that demanded admin to copy one file would be the
; first thing contradicting that.

Unicode true
SetCompressor /SOLID lzma

; NSIS stores each packed file's modification time by default, and git sets
; those to whenever the checkout happened. That made the installer depend on
; when the machine cloned the repository rather than on what was in it, which
; the release's reproducibility check caught on its first real run. Turning it
; off costs nothing: installed files get the install time, which is what a user
; would expect anyway.
SetDateSave off

!include "MUI2.nsh"
!include "FileFunc.nsh"

; Passed in from the build script.
!ifndef VERSION
  !define VERSION "0.0.0"
!endif
!ifndef ARCH
  !define ARCH "amd64"
!endif
!ifndef SOURCE
  !define SOURCE "."
!endif
!ifndef OUTFILE
  !define OUTFILE "SupportOne-Setup.exe"
!endif

!define APPNAME "SupportOne"
!define PUBLISHER "SupportOne"
!define HOMEPAGE "https://github.com/ZanOzair/SupportOne"
!define UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APPNAME}"

Name "${APPNAME} ${VERSION}"
OutFile "${OUTFILE}"
RequestExecutionLevel user
InstallDir "$LOCALAPPDATA\Programs\${APPNAME}"
InstallDirRegKey HKCU "Software\${APPNAME}" "InstallDir"
ShowInstDetails show
ShowUnInstDetails show

VIProductVersion "${VERSION}.0"
VIAddVersionKey "ProductName" "${APPNAME}"
VIAddVersionKey "FileDescription" "${APPNAME} installer"
VIAddVersionKey "FileVersion" "${VERSION}.0"
VIAddVersionKey "ProductVersion" "${VERSION}.0"
VIAddVersionKey "CompanyName" "${PUBLISHER}"
VIAddVersionKey "LegalCopyright" "Apache-2.0 licensed."

!define MUI_ICON "supportone.ico"
!define MUI_UNICON "supportone.ico"
!define MUI_ABORTWARNING

!define MUI_WELCOMEPAGE_TITLE "Install ${APPNAME} ${VERSION}"
!define MUI_WELCOMEPAGE_TEXT "SupportOne looks at this computer and explains, in plain language, what it finds.$\r$\n$\r$\nIt reads only. It changes nothing unless you confirm the exact change, and it sends nothing anywhere unless you click to send it. There is no telemetry.$\r$\n$\r$\nThis installs for you only, in your own user folder, so Windows will not ask for administrator rights."

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${SOURCE}\LICENSE"
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES

!define MUI_FINISHPAGE_RUN "$INSTDIR\supportone-agent.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Check this computer now"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\START-HERE.txt"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "Read the getting-started notes"
!define MUI_FINISHPAGE_SHOWREADME_NOTCHECKED
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "English"

Section "SupportOne" SecMain
  SectionIn RO
  SetOutPath "$INSTDIR"

  File "${SOURCE}\supportone-agent.exe"
  File "${SOURCE}\LICENSE"
  File "${SOURCE}\README.md"
  File "${SOURCE}\START-HERE.txt"
  File "supportone.ico"

  WriteRegStr HKCU "Software\${APPNAME}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "Software\${APPNAME}" "Version" "${VERSION}"

  ; Registered so it appears in Settings > Apps and can be removed the same
  ; way as anything else. A program that cannot be uninstalled through the
  ; normal route is one people are right to distrust.
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayName" "${APPNAME}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTKEY}" "DisplayIcon" "$INSTDIR\supportone.ico"
  WriteRegStr HKCU "${UNINSTKEY}" "Publisher" "${PUBLISHER}"
  WriteRegStr HKCU "${UNINSTKEY}" "URLInfoAbout" "${HOMEPAGE}"
  WriteRegStr HKCU "${UNINSTKEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTKEY}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
  WriteRegStr HKCU "${UNINSTKEY}" "QuietUninstallString" '"$INSTDIR\Uninstall.exe" /S'
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTKEY}" "NoRepair" 1

  ${GetSize} "$INSTDIR" "/S=0K" $0 $1 $2
  IntFmt $0 "0x%08X" $0
  WriteRegDWORD HKCU "${UNINSTKEY}" "EstimatedSize" "$0"

  CreateDirectory "$SMPROGRAMS\${APPNAME}"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk" "$INSTDIR\supportone-agent.exe" "" "$INSTDIR\supportone.ico"
  CreateShortcut "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk" "$INSTDIR\Uninstall.exe"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Section "Desktop shortcut" SecDesktop
  CreateShortcut "$DESKTOP\${APPNAME}.lnk" "$INSTDIR\supportone-agent.exe" "" "$INSTDIR\supportone.ico"
SectionEnd

LangString DESC_SecMain ${LANG_ENGLISH} "The SupportOne program and its documentation."
LangString DESC_SecDesktop ${LANG_ENGLISH} "Put a SupportOne icon on the desktop."

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
  !insertmacro MUI_DESCRIPTION_TEXT ${SecMain} $(DESC_SecMain)
  !insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} $(DESC_SecDesktop)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

Section "Uninstall"
  Delete "$INSTDIR\supportone-agent.exe"
  Delete "$INSTDIR\LICENSE"
  Delete "$INSTDIR\README.md"
  Delete "$INSTDIR\START-HERE.txt"
  Delete "$INSTDIR\supportone.ico"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"

  Delete "$SMPROGRAMS\${APPNAME}\${APPNAME}.lnk"
  Delete "$SMPROGRAMS\${APPNAME}\Uninstall ${APPNAME}.lnk"
  RMDir "$SMPROGRAMS\${APPNAME}"
  Delete "$DESKTOP\${APPNAME}.lnk"

  DeleteRegKey HKCU "${UNINSTKEY}"
  DeleteRegKey HKCU "Software\${APPNAME}"

  ; The audit log and any saved reports are the user's own records and are
  ; deliberately left alone. Removing the program should not remove the
  ; evidence of what it did.
SectionEnd

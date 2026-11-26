Unicode True
RequestExecutionLevel user
Name "SafeRM"
!ifndef PRODUCT_VERSION
  !define PRODUCT_VERSION "0.1.0"
!endif
VIProductVersion "${PRODUCT_VERSION}.0"
VIAddVersionKey /LANG=1033 "ProductName" "SafeRM"
VIAddVersionKey /LANG=1033 "FileDescription" "SafeRM installer"
VIAddVersionKey /LANG=1033 "FileVersion" "${PRODUCT_VERSION}"
VIAddVersionKey /LANG=1033 "LegalCopyright" "Shulem Deen"
OutFile "${BUILD_DIR}\saferm-app-windows-${TARGET_ARCH}-setup.exe"
InstallDir "$LOCALAPPDATA\Programs\SafeRM"
InstallDirRegKey HKCU "Software\SafeRM" "InstallDirectory"
!include "MUI2.nsh"
!include "LogicLib.nsh"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "English"

Section "SafeRM"
  SetShellVarContext current
  StrCpy $1 2
  ClearErrors
  IfFileExists "$INSTDIR\saferm-notify.exe" 0 +2
    ExecWait '"$INSTDIR\saferm-notify.exe" --quit' $1
  ${If} ${Errors}
    StrCpy $1 -1
  ${EndIf}
  ${If} $1 != 0
  ${AndIf} $1 != 2
    MessageBox MB_ICONSTOP "SafeRM Notify could not be stopped. Close it before continuing the installation." /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
  SetOutPath "$INSTDIR"
  File "${BUILD_DIR}\saferm-app.exe"
  File "${BUILD_DIR}\saferm-notify.exe"
  File "${BUILD_DIR}\saferm.exe"
  File "path.ps1"
  SetRegView 64
  ReadRegStr $0 HKLM "SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    ReadRegStr $0 HKCU "Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}" "pv"
  ${EndIf}
  ${If} $0 == ""
  ${OrIf} $0 == "0.0.0.0"
    InitPluginsDir
    SetOutPath "$PLUGINSDIR"
    File "${BUILD_DIR}\MicrosoftEdgeWebview2Setup.exe"
    ClearErrors
    ExecWait '"$PLUGINSDIR\MicrosoftEdgeWebview2Setup.exe" /silent /install' $0
    ${If} ${Errors}
      StrCpy $0 -1
    ${EndIf}
    ${If} $0 != 0
      MessageBox MB_ICONSTOP "WebView2 installation failed ($0). Install the Microsoft Edge WebView2 Runtime, then run this installer again." /SD IDOK
      SetErrorLevel 1
      Abort
    ${EndIf}
    SetOutPath "$INSTDIR"
  ${EndIf}
  WriteRegStr HKCU "Software\SafeRM" "InstallDirectory" "$INSTDIR"
  WriteUninstaller "$INSTDIR\Uninstall.exe"
  CreateShortcut "$SMPROGRAMS\SafeRM.lnk" "$INSTDIR\saferm-app.exe"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM" "DisplayName" "SafeRM"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM" "DisplayVersion" "${PRODUCT_VERSION}"
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM" "DisplayIcon" "$INSTDIR\saferm-app.exe"
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM" "NoModify" 1
  WriteRegDWORD HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM" "NoRepair" 1
  ClearErrors
  ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\path.ps1" Add "$INSTDIR"' $0
  ${If} ${Errors}
  ${OrIf} $0 != 0
    SetErrorLevel 1
    Abort
  ${EndIf}
  SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  ${If} $1 == 0
    ClearErrors
    Exec '"$INSTDIR\saferm-notify.exe"'
    ${If} ${Errors}
      MessageBox MB_ICONEXCLAMATION "SafeRM was installed, but SafeRM Notify could not be restarted." /SD IDOK
    ${EndIf}
  ${EndIf}
SectionEnd

Section "Uninstall"
  SetShellVarContext current
  SetRegView 64
  StrCpy $1 2
  ClearErrors
  IfFileExists "$INSTDIR\saferm-notify.exe" 0 +2
    ExecWait '"$INSTDIR\saferm-notify.exe" --quit' $1
  ${If} ${Errors}
    StrCpy $1 -1
  ${EndIf}
  ${If} $1 != 0
  ${AndIf} $1 != 2
    MessageBox MB_ICONSTOP "SafeRM Notify could not be stopped. Close it before continuing the uninstall." /SD IDOK
    SetErrorLevel 1
    Abort
  ${EndIf}
  ClearErrors
  ExecWait '"$SYSDIR\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "$INSTDIR\path.ps1" Remove "$INSTDIR"' $0
  ${If} ${Errors}
  ${OrIf} $0 != 0
    SetErrorLevel 1
    Abort
  ${EndIf}
  SendMessage ${HWND_BROADCAST} ${WM_SETTINGCHANGE} 0 "STR:Environment" /TIMEOUT=5000
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "com.shdeen.saferm.notify"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\SafeRM"
  DeleteRegKey HKCU "Software\SafeRM"
  Delete "$SMPROGRAMS\SafeRM.lnk"
  Delete "$PROFILE\.saferm\notifications.setup"
  Delete "$INSTDIR\saferm-app.exe"
  Delete "$INSTDIR\saferm-notify.exe"
  Delete "$INSTDIR\saferm.exe"
  Delete "$INSTDIR\path.ps1"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
SectionEnd

; Per-user installer for the ctxhop GUI vNext release package (Inno Setup 6.7).
; Build: ISCC.exe /DStage=<extracted release zip>\ctxhop-gui-vnext /DTag=<release date tag> CtxHop-GUI-vNext.iss
; User data lives in %LOCALAPPDATA%\CtxHopGUI and is never touched by setup or uninstall.
#ifndef Stage
  #error Pass /DStage=<extracted release zip>\ctxhop-gui-vnext
#endif
#ifndef Tag
  #error Pass /DTag=<release date tag>, for example /DTag=20260927.1
#endif
; Same command as Run-CtxHop-GUI-vNext.cmd, started minimized so no console window flashes.
; Setup stays in 32-bit mode like the first release so upgrades keep one uninstall log; the shortcut path is
; resolved by 64-bit Explorer and the [Run] entry uses the 64bit flag, so both start 64-bit Windows PowerShell.
#define PowerShell "{win}\System32\WindowsPowerShell\v1.0\powershell.exe"
; Single quotes keep the doubled quotes that the [Icons]/[Run] parameter syntax needs around the path.
#define GuiArgs '-NoLogo -NoProfile -STA -WindowStyle Hidden -ExecutionPolicy RemoteSigned -File ""{app}\GUI.ps1""'

[Setup]
AppId={{2CC6D235-90EE-48E7-9059-3FFC393D8C9E}
AppName=CtxHop GUI vNext
AppVersion={#Tag}
AppPublisher=jaeseongs95
AppPublisherURL=https://github.com/jaeseongs95/ctxhop
AppUpdatesURL=https://github.com/jaeseongs95/ctxhop/releases
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
DefaultDirName={autopf}\CtxHop GUI vNext
; A fixed folder keeps the install away from the data folder.
DisableDirPage=yes
DisableProgramGroupPage=yes
; Every GUI worker task (list, backup, restore, setup...) holds this mutex; the stable ctxhop-gui uses the same name.
AppMutex=CtxHopGUI-operation
; Never let Restart Manager close ctxhop.exe in the middle of a task.
CloseApplications=no
OutputDir=out
OutputBaseFilename=CtxHop-GUI-vNext-{#Tag}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
UninstallDisplayName=CtxHop GUI vNext
UninstallDisplayIcon={#PowerShell}

[Languages]
Name: "korean"; MessagesFile: "compiler:Languages\Korean.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "{#Stage}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{autoprograms}\CtxHop GUI vNext"; Filename: "{#PowerShell}"; Parameters: "{#GuiArgs}"; WorkingDir: "{app}"; Flags: runminimized
Name: "{autodesktop}\CtxHop GUI vNext"; Filename: "{#PowerShell}"; Parameters: "{#GuiArgs}"; WorkingDir: "{app}"; Flags: runminimized; Tasks: desktopicon

[Run]
Filename: "{#PowerShell}"; Parameters: "{#GuiArgs}"; WorkingDir: "{app}"; Description: "{cm:LaunchProgram,CtxHop GUI vNext}"; Flags: postinstall nowait skipifsilent runminimized 64bit

[UninstallDelete]
; Python bytecode cache only; a user-made backend\runtime.json stays.
Type: filesandordirs; Name: "{app}\backend\__pycache__"

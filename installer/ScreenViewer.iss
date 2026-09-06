#define MyAppName "ScreenViewer"
#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

#ifndef AppSource
  #error AppSource define is required
#endif

#ifndef ChromeExtSource
  #error ChromeExtSource define is required
#endif

#ifndef RuntimeInstaller
  #error RuntimeInstaller define is required
#endif

#ifndef OutputDir
  #define OutputDir "."
#endif

[Setup]
AppId={{F1F80E88-2B2D-4EC7-89B2-41E10D1B6D11}
AppName={#MyAppName}
AppVersion={#AppVersion}
AppPublisher=ScreenViewer
DefaultDirName={autopf}\ScreenViewer
DefaultGroupName=ScreenViewer
DisableProgramGroupPage=yes
OutputDir={#OutputDir}
OutputBaseFilename=screenviewer-installer
Compression=lzma
SolidCompression=yes
ArchitecturesAllowed=x64
ArchitecturesInstallIn64BitMode=x64
PrivilegesRequired=admin
WizardStyle=modern
UninstallDisplayIcon={app}\ScreenViewer.exe

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional options:"; Flags: unchecked
Name: "chromeext"; Description: "Prepare Chrome extension files and open setup instructions"; GroupDescription: "Additional options:"; Flags: unchecked

[Files]
Source: "{#AppSource}\*"; DestDir: "{app}"; Flags: recursesubdirs createallsubdirs ignoreversion
Source: "{#RuntimeInstaller}"; DestDir: "{tmp}"; Flags: deleteafterinstall
Source: "{#ChromeExtSource}\*"; DestDir: "{localappdata}\ScreenViewer\chrome-extension"; Flags: recursesubdirs createallsubdirs ignoreversion; Tasks: chromeext
Source: "ChromeExtension-Install.txt"; DestDir: "{localappdata}\ScreenViewer\chrome-extension"; Flags: ignoreversion; Tasks: chromeext

[Icons]
Name: "{autoprograms}\ScreenViewer"; Filename: "{app}\ScreenViewer.exe"
Name: "{autodesktop}\ScreenViewer"; Filename: "{app}\ScreenViewer.exe"; Tasks: desktopicon

[Run]
Filename: "{tmp}\WindowsAppRuntimeInstall-x64.exe"; Parameters: "--quiet"; Flags: waituntilterminated
Filename: "chrome://extensions/"; Flags: shellexec postinstall skipifsilent; Tasks: chromeext
Filename: "notepad.exe"; Parameters: "\"{localappdata}\ScreenViewer\chrome-extension\ChromeExtension-Install.txt\""; Flags: postinstall skipifsilent; Tasks: chromeext
Filename: "{app}\ScreenViewer.exe"; Description: "Launch ScreenViewer"; Flags: nowait postinstall skipifsilent

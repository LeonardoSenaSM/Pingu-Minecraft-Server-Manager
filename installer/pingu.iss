#define MyAppName "Pingu"
#define MyAppPublisher "Pingu Minecraft Server Manager"
#define MyAppExeName "pingu.exe"
#define MyAppVersion GetEnv("PINGU_VERSION")

[Setup]
AppId={{9AA732C2-5349-48CA-9411-6BE454B826B3}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}
DefaultDirName={autopf}\Pingu Minecraft Server Manager
DefaultGroupName=Pingu Minecraft Server Manager
DisableProgramGroupPage=no
OutputDir=..\dist
OutputBaseFilename=Pingu-Setup-{#MyAppVersion}-Windows-x64
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0.17763
UninstallDisplayIcon={app}\{#MyAppExeName}
SetupLogging=yes
CloseApplications=yes
RestartApplications=no

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"
Name: "brazilianportuguese"; MessagesFile: "compiler:Languages\BrazilianPortuguese.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut / Criar um atalho na Área de Trabalho"; GroupDescription: "Additional shortcuts / Atalhos adicionais:"

[Files]
Source: "..\dist\app\pingu.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\app\runtime\*"; DestDir: "{app}\runtime"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "..\dist\app\README.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\app\README.pt-BR.md"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\dist\app\LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Pingu"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"
Name: "{group}\Uninstall Pingu / Desinstalar Pingu"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Pingu"; Filename: "{app}\{#MyAppExeName}"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "Launch Pingu / Executar o Pingu"; WorkingDir: "{app}"; Flags: nowait postinstall skipifsilent runasoriginaluser

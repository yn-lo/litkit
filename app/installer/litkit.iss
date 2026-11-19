; litkit Windows 安装包（Inno Setup 6）
;
; 构建（先产出 dist\litkit-amd64.exe / dist\litkit-arm64.exe）：
;   ISCC.exe installer\litkit.iss /DAppVersion=0.1.0 /DAppArch=amd64
; 产物：app\dist\litkit-<version>-windows-<arch>-setup.exe
;
; 特性：当前用户安装（免 UAC）、写入 HKCU PATH、同 AppId 覆盖升级。
; 配置随项目：首次使用在项目目录执行 litkit init 生成项目级 .env（不创建全局配置）。

#define AppName "litkit"
#define AppPublisher "litkit"

#ifndef AppVersion
  #define AppVersion "dev"
#endif
#ifndef AppArch
  #define AppArch "amd64"
#endif
#ifndef SourceDir
  #define SourceDir "..\dist"
#endif

[Setup]
; 固定 AppId：升级识别与卸载项依赖它，禁止修改
AppId={{9F2C4B7D-3E6A-4C1B-9D8E-7A5F0B2C4D61}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={localappdata}\Programs\litkit
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
UsePreviousAppDir=yes
ChangesEnvironment=yes
OutputDir={#SourceDir}
OutputBaseFilename=litkit-{#AppVersion}-windows-{#AppArch}-setup
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes
UninstallDisplayIcon={app}\litkit.exe

#if AppArch == "arm64"
ArchitecturesAllowed=arm64
ArchitecturesInstallIn64BitMode=arm64
#else
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
#endif

[Tasks]
Name: "addtopath"; Description: "将 litkit 加入 PATH（当前用户）"; GroupDescription: "环境变量:"

[Files]
Source: "{#SourceDir}\litkit-{#AppArch}.exe"; DestDir: "{app}"; DestName: "litkit.exe"; Flags: ignoreversion

[Registry]
; {olddata} 追加到已有 PATH；NeedsAddPath 避免重复
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; \
  ValueData: "{olddata};{app}"; Tasks: addtopath; Check: NeedsAddPath('{app}')

[Code]
const
  EnvKey = 'Environment';

function NeedsAddPath(Param: string): Boolean;
var
  OrigPath, Dir: string;
begin
  Dir := ExpandConstant(Param);
  if not RegQueryStringValue(HKEY_CURRENT_USER, EnvKey, 'Path', OrigPath) then
  begin
    Result := True;
    Exit;
  end;
  Result := Pos(';' + Uppercase(Dir) + ';', ';' + Uppercase(OrigPath) + ';') = 0;
end;

procedure RemoveFromPath(Dir: string);
var
  Paths, NewPaths, Item: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, EnvKey, 'Path', Paths) then
    Exit;
  Paths := Paths + ';';
  P := Pos(';', Paths);
  while P > 0 do
  begin
    Item := Trim(Copy(Paths, 1, P - 1));
    if (Item <> '') and (CompareText(Item, Dir) <> 0) then
    begin
      if NewPaths <> '' then
        NewPaths := NewPaths + ';';
      NewPaths := NewPaths + Item;
    end;
    Delete(Paths, 1, P);
    P := Pos(';', Paths);
  end;
  RegWriteExpandStringValue(HKEY_CURRENT_USER, EnvKey, 'Path', NewPaths);
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveFromPath(ExpandConstant('{app}'));
end;

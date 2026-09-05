#ifndef AppVersion
  #error AppVersion must be provided with /DAppVersion=<version>
#endif
#ifndef StageDir
  #error StageDir must be provided with /DStageDir=<absolute-path>
#endif
#ifndef OutputDir
  #error OutputDir must be provided with /DOutputDir=<absolute-path>
#endif

#define AppName "Locus"

[Setup]
AppId={{B7B7624C-E4F7-4A06-A9AD-7B30136B3178}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=locus-scope
DefaultDirName={%USERPROFILE}\.locus
DefaultGroupName=Locus
DisableProgramGroupPage=yes
DisableDirPage=yes
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
Compression=lzma2
SolidCompression=yes
OutputDir={#OutputDir}
OutputBaseFilename=locus-setup-windows-amd64
UninstallFilesDir={app}\installer
ChangesEnvironment=yes
SetupLogging=yes
WizardStyle=modern
CloseApplications=yes
RestartApplications=no

[Types]
Name: "full"; Description: "完整安装"
Name: "compact"; Description: "仅安装 Locus Scope CLI"
Name: "custom"; Description: "自定义安装"; Flags: iscustom

[Components]
Name: "scope"; Description: "Locus Scope CLI"; Types: full compact custom
Name: "pkg"; Description: "Locus Package CLI"; Types: full custom

[Tasks]
Name: "addpath"; Description: "将 {%USERPROFILE}\.locus\bin 添加到当前用户 PATH"; Flags: checkedonce

[Files]
Source: "{#StageDir}\bin\locus-scope.exe"; DestDir: "{app}\bin"; Components: scope; Flags: ignoreversion
Source: "{#StageDir}\bin\locus-pkg.exe"; DestDir: "{app}\bin"; Components: pkg; Flags: ignoreversion
Source: "{#StageDir}\licenses\locus-license.txt"; DestDir: "{app}\licenses"; Flags: ignoreversion

[Icons]
Name: "{group}\卸载 Locus"; Filename: "{uninstallexe}"

[InstallDelete]
Type: files; Name: "{app}\bin\locus-scope.exe"; Check: ShouldRemoveScope
Type: files; Name: "{app}\bin\locus-pkg.exe"; Check: ShouldRemovePkg

[UninstallDelete]
Type: dirifempty; Name: "{app}\bin"
Type: dirifempty; Name: "{app}\licenses"

[Code]
function HasSelectedCli: Boolean;
begin
  Result := WizardIsComponentSelected('scope') or WizardIsComponentSelected('pkg');
end;

function ShouldRemoveScope: Boolean;
begin
  Result := not WizardIsComponentSelected('scope');
end;

function ShouldRemovePkg: Boolean;
begin
  Result := not WizardIsComponentSelected('pkg');
end;

function CanonicalPathEntry(Value: String): String;
var
  UserProfile: String;
begin
  Result := Lowercase(Trim(Value));
  if (Length(Result) >= 2) and (Result[1] = '"') and
     (Result[Length(Result)] = '"') then
  begin
    Delete(Result, Length(Result), 1);
    Delete(Result, 1, 1);
  end;
  StringChangeEx(Result, '/', '\', True);
  UserProfile := Lowercase(ExpandConstant('{%USERPROFILE}'));
  StringChangeEx(Result, '%userprofile%', UserProfile, True);
  while (Length(Result) > 3) and (Result[Length(Result)] = '\') do
    Delete(Result, Length(Result), 1);
end;

function IsLocusBinPath(Value: String): Boolean;
begin
  Result := CanonicalPathEntry(Value) =
    CanonicalPathEntry(ExpandConstant('{app}\bin'));
end;

procedure SplitPathEntries(Value: String; var Entries: TArrayOfString);
var
  Index: Integer;
  SegmentStart: Integer;
  EntryCount: Integer;
begin
  SetArrayLength(Entries, 0);
  SegmentStart := 1;
  EntryCount := 0;
  for Index := 1 to Length(Value) + 1 do
  begin
    if Index > Length(Value) then
    begin
      SetArrayLength(Entries, EntryCount + 1);
      Entries[EntryCount] := Copy(Value, SegmentStart, Index - SegmentStart);
      EntryCount := EntryCount + 1;
    end
    else if Value[Index] = ';' then
    begin
      SetArrayLength(Entries, EntryCount + 1);
      Entries[EntryCount] := Copy(Value, SegmentStart, Index - SegmentStart);
      EntryCount := EntryCount + 1;
      SegmentStart := Index + 1;
    end;
  end;
end;

function JoinPathEntries(Entries: TArrayOfString): String;
var
  Index: Integer;
begin
  Result := '';
  for Index := 0 to GetArrayLength(Entries) - 1 do
  begin
    if Index > 0 then
      Result := Result + ';';
    Result := Result + Entries[Index];
  end;
end;

procedure AddLocusBinToPath;
var
  ExistingPath: String;
  Entries: TArrayOfString;
  Index: Integer;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', ExistingPath) then
    ExistingPath := '';
  SplitPathEntries(ExistingPath, Entries);
  for Index := 0 to GetArrayLength(Entries) - 1 do
    if IsLocusBinPath(Entries[Index]) then
      Exit;

  if ExistingPath = '' then
    ExistingPath := ExpandConstant('{app}\bin')
  else if ExistingPath[Length(ExistingPath)] = ';' then
    ExistingPath := ExistingPath + ExpandConstant('{app}\bin')
  else
    ExistingPath := ExistingPath + ';' + ExpandConstant('{app}\bin');
  RegWriteExpandStringValue(HKCU, 'Environment', 'Path', ExistingPath);
end;

procedure RemoveLocusBinFromPath;
var
  ExistingPath: String;
  Entries: TArrayOfString;
  KeptEntries: TArrayOfString;
  Index: Integer;
  KeptCount: Integer;
  UpdatedPath: String;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', ExistingPath) then
    Exit;

  SplitPathEntries(ExistingPath, Entries);
  SetArrayLength(KeptEntries, GetArrayLength(Entries));
  KeptCount := 0;
  for Index := 0 to GetArrayLength(Entries) - 1 do
  begin
    if not IsLocusBinPath(Entries[Index]) then
    begin
      KeptEntries[KeptCount] := Entries[Index];
      KeptCount := KeptCount + 1;
    end;
  end;
  SetArrayLength(KeptEntries, KeptCount);
  UpdatedPath := JoinPathEntries(KeptEntries);
  if UpdatedPath = ExistingPath then
    Exit;
  if UpdatedPath = '' then
    RegDeleteValue(HKCU, 'Environment', 'Path')
  else
    RegWriteExpandStringValue(HKCU, 'Environment', 'Path', UpdatedPath);
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if (CurPageID = wpSelectComponents) and not HasSelectedCli then
  begin
    MsgBox('请至少选择一个 CLI 组件。', mbError, MB_OK);
    Result := False;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep <> ssPostInstall then
    Exit;

  if WizardIsTaskSelected('addpath') and HasSelectedCli then
    AddLocusBinToPath
  else
    RemoveLocusBinFromPath;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveLocusBinFromPath;
end;

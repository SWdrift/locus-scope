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
#define PowerShellPath "{sys}\WindowsPowerShell\v1.0\powershell.exe"

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
Name: "compact"; Description: "仅安装 Locus CLI"
Name: "custom"; Description: "自定义安装"; Flags: iscustom

[Components]
Name: "scope"; Description: "Locus Scope CLI"; Types: full compact custom
Name: "pkg"; Description: "Locus Package CLI"; Types: full compact custom
Name: "zot"; Description: "Local OCI Registry (Zot)"; Types: full custom

[Tasks]
Name: "addpath"; Description: "将 {%USERPROFILE}\.locus\bin 添加到当前用户 PATH"; Flags: checkedonce; Check: HasSelectedCli
Name: "zotautostart"; Description: "登录 Windows 后自动启动 Zot"; Components: zot; Flags: unchecked

[Dirs]
Name: "{app}\zot\logs"; Components: zot
Name: "{app}\zot\registry"; Components: zot

[Files]
Source: "{#StageDir}\bin\locus-scope.exe"; DestDir: "{app}\bin"; Components: scope; Flags: ignoreversion
Source: "{#StageDir}\bin\locus-pkg.exe"; DestDir: "{app}\bin"; Components: pkg; Flags: ignoreversion
Source: "{#StageDir}\zot\zot.exe"; DestDir: "{app}\zot\bin"; Components: zot; Flags: ignoreversion
Source: "{#StageDir}\libexec\locus-zot-user.ps1"; DestDir: "{app}\libexec"; Components: zot; Flags: ignoreversion
Source: "{#StageDir}\licenses\locus-license.txt"; DestDir: "{app}\licenses"; Flags: ignoreversion
Source: "{#StageDir}\licenses\zot-license.txt"; DestDir: "{app}\licenses"; Components: zot; Flags: ignoreversion

[Icons]
Name: "{group}\启动 Zot"; Filename: "{#PowerShellPath}"; Parameters: "-NoLogo -NoProfile -ExecutionPolicy Bypass -File ""{app}\libexec\locus-zot-user.ps1"" start"; WorkingDir: "{app}\zot"; Components: zot
Name: "{group}\停止 Zot"; Filename: "{#PowerShellPath}"; Parameters: "-NoExit -NoLogo -NoProfile -ExecutionPolicy Bypass -File ""{app}\libexec\locus-zot-user.ps1"" stop"; WorkingDir: "{app}\zot"; Components: zot
Name: "{group}\查看 Zot 状态"; Filename: "{#PowerShellPath}"; Parameters: "-NoExit -NoLogo -NoProfile -ExecutionPolicy Bypass -File ""{app}\libexec\locus-zot-user.ps1"" status"; WorkingDir: "{app}\zot"; Components: zot
Name: "{group}\打开 Zot 日志目录"; Filename: "{sys}\explorer.exe"; Parameters: """{app}\zot\logs"""; Components: zot
Name: "{group}\卸载 Locus"; Filename: "{uninstallexe}"

[Run]
Filename: "{#PowerShellPath}"; Parameters: "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File ""{app}\libexec\locus-zot-user.ps1"" start"; Description: "立即启动 Zot"; WorkingDir: "{app}\zot"; Components: zot; Flags: postinstall skipifsilent runhidden

[InstallDelete]
Type: files; Name: "{app}\bin\locus-scope.exe"; Check: ShouldRemoveScope
Type: files; Name: "{app}\bin\locus-pkg.exe"; Check: ShouldRemovePkg
Type: files; Name: "{app}\zot\bin\zot.exe"; Check: ShouldRemoveZot
Type: files; Name: "{app}\libexec\locus-zot-user.ps1"; Check: ShouldRemoveZot
Type: files; Name: "{app}\licenses\zot-license.txt"; Check: ShouldRemoveZot
Type: files; Name: "{app}\zot\config.json"; Check: ShouldRemoveZot
Type: files; Name: "{app}\zot\zot.pid"; Check: ShouldRemoveZot
Type: filesandordirs; Name: "{app}\zot\logs"; Check: ShouldRemoveZot
Type: dirifempty; Name: "{app}\zot\bin"; Check: ShouldRemoveZot
Type: dirifempty; Name: "{app}\zot"; Check: ShouldRemoveZot
Type: dirifempty; Name: "{app}\libexec"; Check: ShouldRemoveZot

[UninstallRun]
Filename: "{#PowerShellPath}"; Parameters: "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File ""{app}\libexec\locus-zot-user.ps1"" stop"; RunOnceId: "StopLocusZot"; Flags: runhidden; Check: IsZotManagerInstalled
Filename: "{#PowerShellPath}"; Parameters: "-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File ""{app}\libexec\locus-zot-user.ps1"" disable-autostart"; RunOnceId: "DisableLocusZotAutoStart"; Flags: runhidden; Check: IsZotManagerInstalled

[UninstallDelete]
Type: files; Name: "{app}\zot\config.json"
Type: files; Name: "{app}\zot\zot.pid"
Type: filesandordirs; Name: "{app}\zot\logs"
Type: filesandordirs; Name: "{app}\zot\registry"; Check: ShouldDeleteZotRepository
Type: dirifempty; Name: "{app}\zot\bin"
Type: dirifempty; Name: "{app}\zot"
Type: dirifempty; Name: "{app}\bin"
Type: dirifempty; Name: "{app}\libexec"
Type: dirifempty; Name: "{app}\licenses"

[Code]
var
  DeleteZotRepositoryData: Boolean;
  RestartZotAfterInstall: Boolean;

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

function ShouldRemoveZot: Boolean;
begin
  Result := not WizardIsComponentSelected('zot');
end;

function ShouldDeleteZotRepository: Boolean;
begin
  Result := DeleteZotRepositoryData;
end;

function ZotManagerPath: String;
begin
  Result := ExpandConstant('{app}\libexec\locus-zot-user.ps1');
end;

function IsZotManagerInstalled: Boolean;
begin
  Result := FileExists(ZotManagerPath);
end;

function ZotManagerParameters(Action: String): String;
begin
  Result := '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File ' +
    AddQuotes(ZotManagerPath) + ' ' + Action;
end;

function RunZotManager(Action: String; var ResultCode: Integer): Boolean;
begin
  Result := Exec(
    ExpandConstant('{#PowerShellPath}'),
    ZotManagerParameters(Action),
    ExpandConstant('{app}\zot'),
    SW_HIDE,
    ewWaitUntilTerminated,
    ResultCode);
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
  if (CurPageID = wpSelectComponents) and
     not HasSelectedCli and
     not WizardIsComponentSelected('zot') then
  begin
    MsgBox('请至少选择一个安装组件。', mbError, MB_OK);
    Result := False;
  end;
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ResultCode: Integer;
begin
  Result := '';
  RestartZotAfterInstall := FileExists(ExpandConstant('{app}\zot\zot.pid')) and
    WizardIsComponentSelected('zot');
  if FileExists(ZotManagerPath) and
     FileExists(ExpandConstant('{app}\zot\zot.pid')) then
  begin
    if not RunZotManager('stop', ResultCode) or (ResultCode <> 0) then
    begin
      Result := '无法停止正在运行的 Zot。请手动停止 Zot 后重试。';
      Exit;
    end;
  end;
  if FileExists(ZotManagerPath) and not WizardIsComponentSelected('zot') then
    RunZotManager('disable-autostart', ResultCode);
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep <> ssPostInstall then
    Exit;

  if WizardIsTaskSelected('addpath') and HasSelectedCli then
    AddLocusBinToPath
  else
    RemoveLocusBinFromPath;

  if WizardIsComponentSelected('zot') then
  begin
    if WizardIsTaskSelected('zotautostart') then
      RunZotManager('enable-autostart', ResultCode)
    else
      RunZotManager('disable-autostart', ResultCode);
    if RestartZotAfterInstall then
      RunZotManager('start', ResultCode);
  end;
end;

function InitializeUninstall: Boolean;
var
  Form: TSetupForm;
  PromptLabel: TNewStaticText;
  DeleteCheckBox: TNewCheckBox;
  ConfirmButton: TNewButton;
  CancelButton: TNewButton;
begin
  DeleteZotRepositoryData := False;
  if UninstallSilent then
  begin
    Result := True;
    Exit;
  end;

  Form := CreateCustomForm(ScaleX(420), ScaleY(150), False, False);
  try
    Form.Caption := '卸载 Locus';
    Form.Position := poScreenCenter;

    PromptLabel := TNewStaticText.Create(Form);
    PromptLabel.Parent := Form;
    PromptLabel.Left := ScaleX(16);
    PromptLabel.Top := ScaleY(16);
    PromptLabel.Width := ScaleX(388);
    PromptLabel.AutoSize := False;
    PromptLabel.WordWrap := True;
    PromptLabel.Caption := '默认保留 Zot 仓库数据和 Locus OCI cache。仅在不再需要本地仓库内容时选择删除。';

    DeleteCheckBox := TNewCheckBox.Create(Form);
    DeleteCheckBox.Parent := Form;
    DeleteCheckBox.Left := ScaleX(16);
    DeleteCheckBox.Top := ScaleY(70);
    DeleteCheckBox.Width := ScaleX(388);
    DeleteCheckBox.Caption := '删除 Zot 仓库数据';
    DeleteCheckBox.Checked := False;

    ConfirmButton := TNewButton.Create(Form);
    ConfirmButton.Parent := Form;
    ConfirmButton.Caption := '继续';
    ConfirmButton.ModalResult := mrOk;
    ConfirmButton.Default := True;
    ConfirmButton.Left := Form.ClientWidth - ScaleX(176);
    ConfirmButton.Top := ScaleY(108);
    ConfirmButton.Width := ScaleX(75);

    CancelButton := TNewButton.Create(Form);
    CancelButton.Parent := Form;
    CancelButton.Caption := '取消';
    CancelButton.ModalResult := mrCancel;
    CancelButton.Cancel := True;
    CancelButton.Left := Form.ClientWidth - ScaleX(91);
    CancelButton.Top := ScaleY(108);
    CancelButton.Width := ScaleX(75);

    Result := Form.ShowModal = mrOk;
    if Result then
      DeleteZotRepositoryData := DeleteCheckBox.Checked;
  finally
    Form.Free;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
    RemoveLocusBinFromPath;
end;

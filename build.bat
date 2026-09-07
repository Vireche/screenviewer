@echo off
setlocal

set "ROOT=%~dp0"
set "APP_PROJECT=%ROOT%src\ScreenViewer.WinUI3\ScreenViewer.WinUI3.csproj"
set "PUBLISH_DIR=%ROOT%artifacts\screenviewer"
set "APP_ZIP=%ROOT%screenviewer-winui3.zip"
set "SETUP_DIR=%ROOT%artifacts\setup"
set "INSTALLER_EXE=%ROOT%screenviewer-installer.exe"
set "INNO_SCRIPT=%ROOT%installer\ScreenViewer.iss"
set "RUNTIME_URL=https://aka.ms/windowsappsdk/2.4/2.4.0/windowsappruntimeinstall-x64.exe"
set "RUNTIME_INSTALLER=%SETUP_DIR%\WindowsAppRuntimeInstall-x64.exe"
set "EXT_DIR=%ROOT%chrome-extension"
set "EXT_ZIP=%ROOT%screenviewer-extension.zip"
set "APP_VERSION=0.0.0-local"
set "INNO_COMPILER="
set "APP_VERSION_SAFE=0.0.0"
set "BUILD_OUT_DIR="

if not "%GITHUB_REF_NAME%"=="" set "APP_VERSION=%GITHUB_REF_NAME%"
if /I "%APP_VERSION:~0,1%"=="v" set "APP_VERSION=%APP_VERSION:~1%"

set "APP_VERSION_SAFE=%APP_VERSION%"
for /f "tokens=1 delims=-+" %%A in ("%APP_VERSION_SAFE%") do set "APP_VERSION_SAFE=%%A"
for /f "delims=0123456789." %%A in ("%APP_VERSION_SAFE%") do set "APP_VERSION_SAFE=0.0.0"
if "%APP_VERSION_SAFE%"=="" set "APP_VERSION_SAFE=0.0.0"

if exist "%PUBLISH_DIR%" rmdir /S /Q "%PUBLISH_DIR%"
if exist "%SETUP_DIR%" rmdir /S /Q "%SETUP_DIR%"

set "APP_DIR=%ROOT%src\ScreenViewer.WinUI3"
if exist "%APP_DIR%\bin" rmdir /S /Q "%APP_DIR%\bin"
if exist "%APP_DIR%\obj" rmdir /S /Q "%APP_DIR%\obj"

dotnet clean "%APP_PROJECT%" -c Release -r win-x64
if errorlevel 1 exit /b %ERRORLEVEL%

dotnet publish "%APP_PROJECT%" -c Release -r win-x64 --self-contained false -o "%PUBLISH_DIR%"
if errorlevel 1 exit /b %ERRORLEVEL%

for /f "delims=" %%D in ('dir /B /AD "%APP_DIR%\bin\Release\net*-windows10.0.19041.0" 2^>nul') do (
	set "BUILD_OUT_DIR=%APP_DIR%\bin\Release\%%D\win-x64"
)

if not defined BUILD_OUT_DIR (
	echo Failed to find build output directory for WinUI resources.
	exit /b 1
)

if not exist "%BUILD_OUT_DIR%\ScreenViewer.pri" (
	echo Missing ScreenViewer.pri in build output: %BUILD_OUT_DIR%
	exit /b 1
)

copy /Y "%BUILD_OUT_DIR%\*.xbf" "%PUBLISH_DIR%\" >nul
if errorlevel 1 (
	echo Failed to copy root XBF resources to publish output.
	exit /b 1
)

copy /Y "%BUILD_OUT_DIR%\*.pri" "%PUBLISH_DIR%\" >nul
if errorlevel 1 (
	echo Failed to copy PRI resources to publish output.
	exit /b 1
)

if exist "%BUILD_OUT_DIR%\Views\*.xbf" (
	if not exist "%PUBLISH_DIR%\Views" mkdir "%PUBLISH_DIR%\Views"
	copy /Y "%BUILD_OUT_DIR%\Views\*.xbf" "%PUBLISH_DIR%\Views\" >nul
	if errorlevel 1 (
		echo Failed to copy view XBF resources to publish output.
		exit /b 1
	)
)

if exist "%APP_ZIP%" del /Q "%APP_ZIP%"
powershell -NoProfile -Command ^
  "Compress-Archive -Path '%PUBLISH_DIR%\*' -DestinationPath '%APP_ZIP%' -Force"
if errorlevel 1 (
	echo Failed to package Screen Viewer build
	exit /b 1
)

mkdir "%SETUP_DIR%"
if errorlevel 1 (
	echo Failed to create setup staging directories
	exit /b 1
)

powershell -NoProfile -Command ^
  "$ProgressPreference='SilentlyContinue'; $ok=$false; for($i=1;$i -le 3;$i++){ try { Invoke-WebRequest -Uri '%RUNTIME_URL%' -OutFile '%RUNTIME_INSTALLER%'; $ok=$true; break } catch { if ($i -eq 3) { throw } Start-Sleep -Seconds 2 } }; if(-not $ok){ throw 'Download failed' }"
if errorlevel 1 (
	echo Failed to download Windows App Runtime installer
	exit /b 1
)

if exist "%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe" set "INNO_COMPILER=%ProgramFiles(x86)%\Inno Setup 6\ISCC.exe"
if not defined INNO_COMPILER if exist "%ProgramFiles%\Inno Setup 6\ISCC.exe" set "INNO_COMPILER=%ProgramFiles%\Inno Setup 6\ISCC.exe"
if not defined INNO_COMPILER if exist "%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe" set "INNO_COMPILER=%LOCALAPPDATA%\Programs\Inno Setup 6\ISCC.exe"
if not defined INNO_COMPILER (
	where /Q ISCC.exe
	if not errorlevel 1 set "INNO_COMPILER=ISCC.exe"
)

if not defined INNO_COMPILER (
	echo Inno Setup compiler was not found. Install Inno Setup 6 or add ISCC.exe to PATH.
	exit /b 1
)

if exist "%INSTALLER_EXE%" del /Q "%INSTALLER_EXE%"
"%INNO_COMPILER%" ^
	/DAppVersion="%APP_VERSION_SAFE%" ^
  /DAppSource="%PUBLISH_DIR%" ^
  /DChromeExtSource="%EXT_DIR%" ^
  /DRuntimeInstaller="%RUNTIME_INSTALLER%" ^
  /DOutputDir="%ROOT%" ^
  "%INNO_SCRIPT%"
if errorlevel 1 (
	echo Failed to build Inno Setup installer
	exit /b 1
)

:: Pack the Chrome extension into a zip
if exist "%EXT_ZIP%" del /Q "%EXT_ZIP%"

powershell -NoProfile -Command ^
  "Compress-Archive -Path '%EXT_DIR%\manifest.json','%EXT_DIR%\background.js' -DestinationPath '%EXT_ZIP%' -Force"
if errorlevel 1 (
	echo Failed to pack Chrome extension
	exit /b 1
)

echo Screen Viewer packaged to screenviewer-winui3.zip
echo Screen Viewer installer created at screenviewer-installer.exe
echo Chrome extension packed to screenviewer-extension.zip
exit /b 0

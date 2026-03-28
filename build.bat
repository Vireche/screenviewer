@echo off
setlocal

set "ROOT=%~dp0"
set "RSRC_SOURCE=%ROOT%resources\windows\rsrc.syso"
set "RSRC_STAGED=%ROOT%rsrc.syso"

copy /Y "%RSRC_SOURCE%" "%RSRC_STAGED%" >nul
if errorlevel 1 (
	echo Failed to stage rsrc.syso from %RSRC_SOURCE%
	exit /b 1
)

go build -ldflags="-H windowsgui" -o screenviewer.exe .
set "BUILD_RC=%ERRORLEVEL%"

del /Q "%RSRC_STAGED%" >nul 2>&1

if %BUILD_RC% neq 0 exit /b %BUILD_RC%

:: Pack the Chrome extension into a zip
set "EXT_DIR=%ROOT%chrome-extension"
set "EXT_ZIP=%ROOT%screenviewer-extension.zip"

if exist "%EXT_ZIP%" del /Q "%EXT_ZIP%"

powershell -NoProfile -Command ^
  "Compress-Archive -Path '%EXT_DIR%\manifest.json','%EXT_DIR%\background.js' -DestinationPath '%EXT_ZIP%' -Force"
if errorlevel 1 (
	echo Failed to pack Chrome extension
	exit /b 1
)

echo Chrome extension packed to screenviewer-extension.zip
exit /b 0

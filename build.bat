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
exit /b %BUILD_RC%

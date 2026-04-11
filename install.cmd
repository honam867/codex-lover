@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install.ps1"
if errorlevel 1 exit /b %errorlevel%
set "PATH=%LOCALAPPDATA%\codex-lover\bin;%PATH%"
echo.
echo Current terminal PATH updated.
echo You can run:
echo   codex-lover

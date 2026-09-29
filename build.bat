@echo off
rem Build the Windows GUI executable without a console window.
rem Find Go on PATH, in D:\golang, or in the user's tools directory.
setlocal
cd /d "%~dp0"
set "GO_EXE=go"
where go >nul 2>nul
if errorlevel 1 (
  if exist "D:\golang\bin\go.exe" (
    set "GO_EXE=D:\golang\bin\go.exe"
  ) else if exist "%USERPROFILE%\tools\go\bin\go.exe" (
    set "GO_EXE=%USERPROFILE%\tools\go\bin\go.exe"
  ) else (
    echo [ERROR] Go not found. Install it from https://go.dev/dl/ and add it to PATH.
    exit /b 1
  )
)
if not exist dist mkdir dist
echo Building with "%GO_EXE%"...
"%GO_EXE%" build -mod=vendor -trimpath -ldflags "-s -w -H windowsgui" -o dist\Hestia.exe .
if errorlevel 1 (
  echo [ERROR] Build failed.
  exit /b 1
)
echo Built: dist\Hestia.exe

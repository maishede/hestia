@echo off
rem Hestia Windows 构建脚本（GUI 版，双击 exe 弹出配置窗口，无控制台黑框）
rem Go 查找顺序：PATH → D:\golang\bin → %USERPROFILE%\tools\go
setlocal
set GO=go
where go >nul 2>nul
if errorlevel 1 (
  if exist "D:\golang\bin\go.exe" (
    set "GO=D:\golang\bin\go.exe"
  ) else if exist "%USERPROFILE%\tools\go\bin\go.exe" (
    set "GO=%USERPROFILE%\tools\go\bin\go.exe"
  ) else (
    echo [错误] 未找到 Go 工具链，请安装到 D:\golang 或从 https://go.dev/dl/ 安装后加入 PATH
    exit /b 1
  )
)
if not exist dist mkdir dist
echo 使用 %GO% 构建中...
%GO% build -trimpath -ldflags "-s -w -H windowsgui" -o dist\Hestia.exe .
if errorlevel 1 ( echo [失败] & exit /b 1 )
echo 构建完成: dist\Hestia.exe（双击打开配置窗口）

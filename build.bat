@echo off
rem Hestia Windows 构建脚本（优先用 PATH 里的 go，其次用 %USERPROFILE%\tools\go）
setlocal
set GO=go
where go >nul 2>nul
if errorlevel 1 (
  if exist "%USERPROFILE%\tools\go\bin\go.exe" (
    set "GO=%USERPROFILE%\tools\go\bin\go.exe"
  ) else (
    echo [错误] 未找到 Go 工具链，请从 https://go.dev/dl/ 安装
    exit /b 1
  )
)
if not exist dist mkdir dist
echo 使用 %GO% 构建中...
%GO% build -trimpath -ldflags "-s -w" -o dist\Hestia.exe .
if errorlevel 1 ( echo [失败] & exit /b 1 )
echo 构建完成: dist\Hestia.exe
echo 直接双击运行即可（配置与数据保存在 exe 同目录）

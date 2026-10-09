@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0.."
if not defined HS_GO set "HS_GO=go"
where "%HS_GO%" >nul 2>nul
if errorlevel 1 (
  echo 未找到 Go 工具链，请设置 HS_GO 为 go.exe 的绝对路径。
  exit /b 1
)
for %%S in (sdkserver loginserver gameserver hotfixserver) do (
  set "GOOS=windows"
  "%HS_GO%" build -o out\bin\windows\%%S.exe .\cmd\%%S
  if errorlevel 1 exit /b 1
)
set "GOOS=linux"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
for %%S in (sdkserver loginserver gameserver hotfixserver) do (
  "%HS_GO%" build -o out\bin\linux-amd64\%%S .\cmd\%%S
  if errorlevel 1 exit /b 1
)
echo Windows 与 Linux 服务构建完成。

@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0.."
if not defined HS_GO set "HS_GO=go"
set "GOOS=windows"
"%HS_GO%" build -o out\bin\windows\gameserver.exe .\cmd\gameserver
if errorlevel 1 exit /b 1
set "GOOS=linux"
set "GOARCH=amd64"
set "CGO_ENABLED=0"
"%HS_GO%" build -o out\bin\linux-amd64\gameserver .\cmd\gameserver
if errorlevel 1 exit /b 1
"%HS_GO%" test -c -o out\bin\linux-amd64\dbstore.test .\internal\game\dbstore
if errorlevel 1 exit /b 1
echo 游戏服务和隔离数据库验收产物构建完成。

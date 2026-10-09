@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0.."
set "HS_GAME_DEBUG_BIND=127.0.0.1:19001"
set "HS_GAME_ADDRESS=127.0.0.1:9000"
set "HS_DATA_DIR=deploy/data"
if /i "%~1"=="game" (
  out\bin\windows\gameserver.exe
) else if /i "%~1"=="hotfix" (
  set "HS_HOTFIX_BIND=0.0.0.0:8082"
  out\bin\windows\hotfixserver.exe
) else if /i "%~1"=="sdk" (
  out\bin\windows\sdkserver.exe
) else if /i "%~1"=="login" (
  out\bin\windows\loginserver.exe
) else (
  echo 用法：deploy\run-local.cmd game、hotfix、sdk 或 login
  exit /b 1
)

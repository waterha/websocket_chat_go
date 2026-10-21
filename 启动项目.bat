@echo off
chcp 65001 >nul
title IM Project Launcher
cd /d "%~dp0"
echo Starting websocket_chat_go ...
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\start.ps1" %*
if errorlevel 1 (
  echo.
  echo Startup failed. Read the messages above, then press any key to close.
  pause >nul
)

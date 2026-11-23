@echo off
setlocal
cd /d "%~dp0.."
if errorlevel 1 exit /b 2
if not exist ".venv\Scripts\python.exe" (
  echo [restore] Python virtual environment unavailable; no database access.
  exit /b 2
)
".venv\Scripts\python.exe" -X utf8 -m qa.operators.restore_legacy_baseline --new-local-restore
exit /b %errorlevel%

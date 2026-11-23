@echo off
setlocal
cd /d "%~dp0.."
if errorlevel 1 exit /b 2
if not exist ".venv\Scripts\python.exe" (
  echo [baseline] Python virtual environment unavailable; no connection attempted.
  exit /b 2
)
".venv\Scripts\python.exe" -X utf8 -m qa.operators.freeze_staging_baseline --staging-readonly
exit /b %errorlevel%

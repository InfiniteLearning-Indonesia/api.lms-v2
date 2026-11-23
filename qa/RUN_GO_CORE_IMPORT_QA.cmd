@echo off
setlocal
cd /d "%~dp0.."
if errorlevel 1 exit /b 2
".venv\Scripts\python.exe" -B -X utf8 -m qa.operators.rehearse_go_core_import
exit /b %errorlevel%

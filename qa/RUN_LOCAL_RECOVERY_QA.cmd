@echo off
setlocal
pushd "%~dp0.."
".venv\Scripts\python.exe" -m qa.local_recovery_run --create-new-local-synthetic-cluster --pg-bin "C:\Program Files\PostgreSQL\18\bin"
set "qa_recovery_result=%errorlevel%"
popd
exit /b %qa_recovery_result%

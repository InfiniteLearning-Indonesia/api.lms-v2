@echo off
setlocal
pushd "%~dp0.."
".venv\Scripts\python.exe" -m qa.age_stream_run --no-download --max-minutes 60
set "qa_protected_result=%errorlevel%"
popd
exit /b %qa_protected_result%

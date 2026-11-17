@echo off
setlocal
pushd "%~dp0.."
".venv\Scripts\python.exe" -m qa.age_stream_run --install-tools --max-minutes 60
set "qa_age_result=%errorlevel%"
popd
exit /b %qa_age_result%

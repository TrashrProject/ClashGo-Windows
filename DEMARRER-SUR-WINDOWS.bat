@echo off
setlocal
cd /d "%~dp0"
where py >nul 2>nul
if errorlevel 1 (
 echo Python n'est pas installe. Installe Python 3.11 ou plus.
 pause
 exit /b 1
)
if not exist ".venv\Scripts\python.exe" py -3 -m venv .venv
call ".venv\Scripts\activate.bat"
python -m pip install -r requirements.txt
if errorlevel 1 (
 echo Installation impossible.
 pause
 exit /b 1
)
if not exist "data" mkdir "data"
if not exist "data\admin_password.txt" (
 python -c "import secrets,pathlib;pathlib.Path('data/admin_password.txt').write_text(secrets.token_urlsafe(24),encoding='utf-8')"
)
set /p GUARDIAN_PASSWORD=<data\admin_password.txt
echo ================================================
echo REQ N GO Manager - AI Guardian (SIMULATION)
echo URL: http://127.0.0.1:8765
echo Utilisateur: admin
echo Mot de passe conserve dans data\admin_password.txt
echo ================================================
python -m uvicorn reqngo.server:app --host 127.0.0.1 --port 8765 --workers 1
pause

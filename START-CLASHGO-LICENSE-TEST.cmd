@echo off
setlocal
cd /d "%~dp0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ".\tools\start-local-license-test.ps1"
if errorlevel 1 (
  echo.
  echo Le test local ClashGO a rencontre une erreur.
  echo Consulte le message ci-dessus.
  pause
  exit /b 1
)
echo.
echo Environnement local ClashGO pret.
pause

@echo off
setlocal EnableExtensions
cd /d "%~dp0"
title REQ N GO - MuMu Live
echo.
echo =============================================
echo         REQ N GO - MUMU LIVE
echo =============================================
echo.
echo Ce lanceur utilise ADB de MuMu sans Android Studio.
echo Laisse MuMu et Clash of Clans ouverts.
echo.

rem Keep a caller-supplied ADB_BINARY. Otherwise prefer MuMu's bundled ADB.
if not defined ADB_BINARY if exist "%ProgramFiles%\Netease\MuMu Player 12\shell\adb.exe" set "ADB_BINARY=%ProgramFiles%\Netease\MuMu Player 12\shell\adb.exe"
if not defined ADB_BINARY if exist "%ProgramFiles%\Netease\MuMu Player 12\nx_main\adb.exe" set "ADB_BINARY=%ProgramFiles%\Netease\MuMu Player 12\nx_main\adb.exe"
if not defined ADB_BINARY if exist "%ProgramFiles%\Netease\MuMuPlayerGlobal-12.0\nx_main\adb.exe" set "ADB_BINARY=%ProgramFiles%\Netease\MuMuPlayerGlobal-12.0\nx_main\adb.exe"
if not defined ADB_BINARY if exist "%ProgramFiles(x86)%\Nemu\vmonitor\bin\adb_server.exe" set "ADB_BINARY=%ProgramFiles(x86)%\Nemu\vmonitor\bin\adb_server.exe"
if not defined ADB_BINARY (
  where adb >nul 2>nul
  if not errorlevel 1 set "ADB_BINARY=adb"
)
if not defined ADB_BINARY (
  echo Aucun ADB detecte automatiquement.
  echo Indique le chemin COMPLET vers adb.exe ou adb_server.exe
  echo par exemple dans le dossier d'installation de MuMu.
  set /p "ADB_BINARY=Chemin de l'executable ADB : "
)
if not defined ADB_BINARY goto :error
"%ADB_BINARY%" version >nul 2>nul
if errorlevel 1 (
  echo Le programme ADB choisi ne fonctionne pas.
  goto :error
)
echo.
echo ADB detecte : "%ADB_BINARY%"
echo MuMu classique : le manuel officiel indique souvent le port 7555.
echo MuMu 12 : les ports peuvent varier selon l'instance.
echo.
set "PORT="
set /p "PORT=Port ADB local a connecter (Entree pour 7555) : "
if not defined PORT set "PORT=7555"
echo Connexion a MuMu sur 127.0.0.1:%PORT%...
"%ADB_BINARY%" connect "127.0.0.1:%PORT%"
echo.
echo Appareils actuellement accessibles :
"%ADB_BINARY%" devices
echo.
echo Si ton appareil n'est pas liste comme device, controle son port ADB.
echo.
call "%~dp0DEMARRER-ANDROID-STUDIO.bat"
goto :end

:error
echo.
echo Echec du demarrage MuMu. Aucun changement fait sur l'emulateur.
pause
:end
endlocal

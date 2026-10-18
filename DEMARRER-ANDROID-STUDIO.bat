@echo off
setlocal EnableExtensions
cd /d "%~dp0"
title REQ N GO - Android Live Studio

echo.
echo ==================================================
echo       REQ N GO - ANDROID LIVE STUDIO
echo ==================================================
echo.
set "PYTHON=python"
if exist ".venv\Scripts\python.exe" set "PYTHON=.venv\Scripts\python.exe"
"%PYTHON%" --version >nul 2>nul
if errorlevel 1 (
  echo ERREUR : Python 3 est introuvable.
  goto :error
)

rem Use official adb if present. BlueStacks 5 bundles HD-Adb.exe in
rem the default installation folder, so no separate SDK download is needed.
set "ADB_CMD=adb"
if defined ADB_BINARY set "ADB_CMD=%ADB_BINARY%"
if not defined ADB_BINARY (
  where adb >nul 2>nul
  if errorlevel 1 (
    if exist "C:\Program Files\BlueStacks_nxt\HD-Adb.exe" (
      set "ADB_CMD=C:\Program Files\BlueStacks_nxt\HD-Adb.exe"
    ) else (
      echo ERREUR : Ni adb ni HD-Adb.exe de BlueStacks 5 ne sont trouves.
      echo Active Android Debug Bridge dans BlueStacks puis installe
      echo les Android Platform Tools ou indique ADB_BINARY.
      goto :error
    )
  )
)
"%ADB_CMD%" version >nul 2>nul
if errorlevel 1 (
  echo ERREUR : L'executable ADB choisi ne fonctionne pas.
  goto :error
)

"%PYTHON%" -c "import cv2, numpy" >nul 2>nul
if errorlevel 1 (
  echo Installation des dependances visuelles OpenCV / NumPy...
  "%PYTHON%" -m pip install -r requirements-android.txt
  if errorlevel 1 goto :error
)

echo ADB utilise : %ADB_CMD%
rem BlueStacks commonly uses local ADB port 5555. The port can be different.
if /I "%ADB_CMD%"=="C:\Program Files\BlueStacks_nxt\HD-Adb.exe" (
  echo Tentative de connexion locale BlueStacks sur 127.0.0.1:5555...
  "%ADB_CMD%" connect 127.0.0.1:5555
)
echo Appareils Android disponibles :
"%PYTHON%" -m reqngo.android.cli devices --adb "%ADB_CMD%"
if errorlevel 1 goto :error

echo.
set "SERIAL="
set /p "SERIAL=Copie ici le serial de TON appareil : "
if not defined SERIAL (
  echo Aucun appareil selectionne.
  goto :error
)
echo.
echo Autoriser un geste tactile uniquement apres validation visuelle ?
choice /c ON /n /m "Appuie sur O pour oui, N pour non : "
set "REQNGO_ANDROID_ACTIONS="
if errorlevel 2 goto :launch
set "REQNGO_ANDROID_ACTIONS=I_ACCEPT_LOCAL_TOUCHES"
:launch
echo.
echo Lancement du studio local sur http://127.0.0.1:8766
echo Ne ferme pas cette fenetre pendant le test.
"%PYTHON%" -m reqngo.android.studio --serial "%SERIAL%" --adb "%ADB_CMD%"
if errorlevel 1 goto :error
goto :end

:error
echo.
echo Le studio ne peut pas demarrer. Verifie Python, ADB et l'appareil.
pause
:end
endlocal

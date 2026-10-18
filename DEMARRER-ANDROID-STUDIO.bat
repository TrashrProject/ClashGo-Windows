@echo off
setlocal EnableExtensions
cd /d "%~dp0"
title REQ N GO - Android Live Studio

echo.
echo ==================================================
echo       REQ N GO - ANDROID LIVE STUDIO
echo ==================================================
echo.
where python >nul 2>nul
if errorlevel 1 (
  echo ERREUR : Python 3 est introuvable dans PATH.
  goto :error
)
where adb >nul 2>nul
if errorlevel 1 (
  echo ERREUR : Android Platform Tools ^(adb^) est introuvable dans PATH.
  echo Installer les outils officiels Android puis relancer.
  goto :error
)

python -c "import cv2, numpy" >nul 2>nul
if errorlevel 1 (
  echo Installation des dependances visuelles OpenCV / NumPy...
  python -m pip install -r requirements-android.txt
  if errorlevel 1 goto :error
)

echo Appareils Android disponibles :
python -m reqngo.android.cli devices
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
python -m reqngo.android.studio --serial "%SERIAL%"
if errorlevel 1 goto :error
goto :end

:error
echo.
echo Le studio ne peut pas demarrer. Verifie Python, ADB et l'appareil.
pause
:end
endlocal

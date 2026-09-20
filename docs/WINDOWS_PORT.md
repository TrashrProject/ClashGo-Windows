# Windows port

This branch is the Windows-first port of ClashGO.

## Current boot path

1. Locate BlueStacks 5 (`HD-Player.exe`).
2. Locate and parse `bluestacks.conf`.
3. Discover instance-specific ADB ports.
4. Reuse an already-running verified BlueStacks device when available.
5. Otherwise start the preferred instance (Tiramisu64 -> Rvc64 -> Pie64 -> Nougat64 -> Nougat32).
6. Connect through ADB and verify the emulator identity.
7. Apply the ClashGO reference display size through `wm size` and `wm density`.
8. Continue into the existing ClashGO boot probe, classifier and recovery ladder.

The upstream macOS backend is preserved. Core boot/recovery now calls the platform-neutral
`EnsureBlueStacks` / `EnsureBlueStacksCtx` API.

## Windows diagnostics

Run from PowerShell:

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\windows-doctor.ps1
```

The doctor reports BlueStacks paths, instances/ADB ports, connected devices, display
size and whether Clash of Clans is installed.

## Optional overrides

- `CLASHGO_BLUESTACKS_PLAYER`: full path to `HD-Player.exe`
- `CLASHGO_BLUESTACKS_HOME`: BlueStacks install directory
- `CLASHGO_BLUESTACKS_DATA`: BlueStacks data directory
- `CLASHGO_BLUESTACKS_CONF`: full path to `bluestacks.conf`
- `CLASHGO_BLUESTACKS_INSTANCE`: force an instance name

These are troubleshooting escape hatches; normal installs should auto-detect.


## Build validation

The `Windows Build` GitHub Actions workflow runs automatically for updates to
`windows/core-port`. It installs the Windows Go/Wails/OpenCV toolchain, restores
the audited upstream runtime assets, runs backend tests, creates the portable
bundle and uploads `ClashGO-v0.6.0-windows-beta-windows.zip` as an artifact.

A successful CI build is the release gate before live BlueStacks testing.


### Multiple BlueStacks instances

ClashGO discovers BlueStacks 5 instances from `bluestacks.conf`. By default it
selects an appropriate instance automatically and probes that instance's ADB
port first. The Settings > Windows Readiness panel can persist a specific
instance when several are installed. Clear the selection to return to automatic
mode. The selected instance is stored as `device.bluestacks_instance` in the
normal ClashGO config and is only changed while the bot is stopped.


## Installer

After the portable bundle has been built, an NSIS setup can be created from the exact same runtime files:

```powershell
choco install nsis -y
powershell -ExecutionPolicy Bypass -File .\\tools\\build-installer.ps1
```

The installer copies the complete tested portable bundle (EXE, assets, OpenCV/MinGW DLLs and update helper) into `%LOCALAPPDATA%\\Programs\\ClashGO Windows`, creates per-user shortcuts and stays writable so one-click self-update works without an administrator prompt. `%APPDATA%\\ClashGO` is preserved on uninstall.


## v0.6 Windows beta readiness

The Windows beta now includes the platform-neutral emulator API, BlueStacks 5
auto-discovery/start/recovery, persistent instance selection, safe ADB enablement,
BlueStacks custom DataDir registry discovery, Windows CPU metrics, portable
runtime packaging, SHA256-verified self-update, readiness diagnostics, support
bundle export, persistent statistics/history, strategy-path rebasing, ADB health
telemetry, recovery outcome telemetry, fail-fast Windows prerequisites and a
full-runtime NSIS installer definition.

Release gate: Windows CI must generate Wails bindings, pass frontend/backend
tests, create the portable bundle and upload the artifact before the branch is
merged into `main`.


## Distribution integrity

Successful Windows CI builds publish the portable ZIP, the NSIS installer and
a `SHA256SUMS.txt` file so downloaded artifacts can be verified independently.


### GoCV Windows link set

CI builds OpenCV 4.13 with the full module set linked by GoCV 0.43 on Windows,
including `xphoto` and `xobjdetect`; the cache key is versioned so an
incomplete native build can never be reused accidentally.


CI generation: v2 — corrected GoCV module set and hardened Windows release pipeline.

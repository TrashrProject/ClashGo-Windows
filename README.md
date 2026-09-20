# ClashGO Windows

Windows-first fork of [ClashGO](https://github.com/DSargent21/ClashGo), based on
the audited upstream `0.5.0-beta` codebase. The original MIT license and
copyright are preserved.

> Current Windows target: **Windows 10/11 x64 + BlueStacks 5**.

## What works in the Windows port

- BlueStacks 5 discovery and startup.
- Multi-instance discovery from `bluestacks.conf`.
- Persistent instance selection from the GUI.
- BlueStacks data-directory discovery through the Windows registry.
- ADB discovery from PATH, Android SDK locations or BlueStacks `HD-Adb.exe`.
- Automatic ADB/device recovery.
- 860×732 / 160-DPI Android display setup.
- Persistent ADB shell for low-latency taps.
- OpenCV / GoCV vision pipeline.
- Search → loot filtering → attack → result → return-home farming loop.
- YAML attack strategies.
- Automatic wall upgrades inherited from upstream.
- Wails/React desktop UI.
- Windows Readiness panel and diagnostic ZIP export.
- Persistent statistics and attack history.
- Windows CPU / ADB health / recovery telemetry.
- Portable ZIP packaging.
- Per-user NSIS installer.
- SHA256-verified GitHub update manifest and in-place Windows updater.

The first beta intentionally focuses on one controlled emulator target before
adding more Windows emulators.

## First run

The packaged release includes `QUICKSTART.md`. The short version is:

1. Install BlueStacks 5.
2. Create and start one BlueStacks instance.
3. Install Clash of Clans in that instance and open it once manually.
4. Start ClashGO.
5. Open **Settings → Windows Readiness**.
6. Select the correct BlueStacks instance if you have more than one.
7. Configure loot thresholds / strategy and press **Start Bot**.

If startup fails, use **Export diagnostics** in ClashGO or run:

```powershell
powershell -ExecutionPolicy Bypass -File .\windows-doctor.ps1
```

The Doctor checks BlueStacks, its configuration, ADB, instance ports, Android
identity/display state and whether `com.supercell.clashofclans` is installed.

## Development setup

```powershell
git clone https://github.com/TrashrProject/ClashGo-Windows.git
cd ClashGo-Windows
git checkout windows/core-port

powershell -ExecutionPolicy Bypass -File .\tools\setup-windows-dev.ps1 -InstallOpenCV
powershell -ExecutionPolicy Bypass -File .\tools\sync-upstream-runtime.ps1
powershell -ExecutionPolicy Bypass -File .\tools\run-windows-dev.ps1
```

The one-time OpenCV setup uses GoCV 0.43 / OpenCV 4.13, builds only the modules
GoCV links on Windows, and uses all available CPU cores.

## Build a portable release

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\build-windows.ps1
```

Output:

- `dist\ClashGO-Windows\`
- `dist\ClashGO-v0.6.0-windows-beta-windows.zip` (version depends on the build argument)

The bundle contains the executable, runtime assets, OpenCV/MinGW DLLs,
Windows Doctor, quickstart documentation and the Windows update helper.

## Build the installer

Install NSIS once, then build from the tested portable runtime:

```powershell
choco install nsis -y
powershell -ExecutionPolicy Bypass -File .\tools\build-installer.ps1
```

The installer is per-user and installs under:

```text
%LOCALAPPDATA%\Programs\TrashrProject\ClashGO Windows
```

Writable data lives separately under:

```text
%APPDATA%\ClashGO
```

Keeping the program directory per-user lets the verified in-place updater
replace the runtime without requiring administrator rights.

## Windows CI

Pull request builds for `windows/core-port` run on `windows-latest` and:

1. restore/build the GoCV-compatible OpenCV runtime;
2. generate Wails TypeScript bindings;
3. build the React frontend;
4. run `go test ./...`;
5. build `ClashGO.exe`;
6. validate the portable runtime;
7. build the NSIS installer;
8. upload both as workflow artifacts.

OpenCV is cached after the first successful native build so later runs avoid the
large one-time compilation cost.

## Releases and updates

The **Windows Release** workflow creates:

- `ClashGO-v<version>-windows.zip`
- `ClashGO-v<version>-windows-setup.exe`
- `latest.json`

`latest.json` contains the Windows asset URL, size and SHA256. ClashGO checks
releases from `TrashrProject/ClashGo-Windows`, verifies the downloaded ZIP,
then uses the bundled PowerShell helper for the in-place update.

## Architecture

```text
BlueStacks 5
   │
   ├─ ADB server / HD-Adb
   │    ├─ direct screencap transport
   │    └─ persistent shell for input
   │
ClashGO (Go)
   ├─ boot / recovery orchestrator
   ├─ OpenCV / GoCV vision
   ├─ game state classifier
   ├─ loot recognition
   ├─ strategy + attack executor
   ├─ wall upgrade automation
   └─ stats / diagnostics / updater
        │
        └─ Wails + React UI
```

## Strategies

Runtime strategies live in `assets/strategies`. The current example is
`auto_edrag_rush.yaml` with its matching formula JSON. Strategy paths stored in
config are normalized to the packaged assets directory, so moving the portable
folder does not invalidate the selected strategy.

## Diagnostics and logs

Useful state is stored under `%APPDATA%\ClashGO`, including logs, statistics,
attack history and exported diagnostics. Diagnostic ZIPs deliberately exclude
screenshots, emulator userdata, credentials and account tokens.

## Upstream

Original project: [DSargent21/ClashGo](https://github.com/DSargent21/ClashGo)

The Windows port keeps upstream runtime assets and attack logic synchronized
from the audited baseline while Windows-specific lifecycle, packaging,
diagnostics and release work lives in this fork.

## License

MIT. See [LICENSE](LICENSE).

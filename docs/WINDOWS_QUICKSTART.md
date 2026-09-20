# ClashGO Windows — Quick Start

## 1. Prerequisites

- Windows 10 or Windows 11, 64-bit.
- BlueStacks 5 installed.
- One BlueStacks instance created and launched at least once.
- Clash of Clans installed inside that instance and able to reach the village normally.

ClashGO targets 860×732 at 160 DPI for the first Windows beta. The application
will configure the Android display through ADB during startup.

## 2. First launch

1. Launch BlueStacks once and wait until Android is fully loaded.
2. Launch Clash of Clans manually once, complete any game update/login prompt,
   and make sure the village is visible.
3. Start `ClashGO.exe`.
4. Open **Settings → Windows Readiness**.
5. Confirm Runtime assets, BlueStacks 5 and ADB are detected.
6. If you have multiple BlueStacks instances, select the one containing Clash
   of Clans. Otherwise leave **Automatic** selected.
7. Configure the loot thresholds / strategy, then press **Start Bot**.

## 3. If startup fails

ClashGO shows the startup failure at the top of the application. Use
**Export diagnostics** from the error banner or Settings. You can also run:

```powershell
powershell -ExecutionPolicy Bypass -File .\windows-doctor.ps1
```

The doctor checks BlueStacks, its configuration, ADB ports, the online Android
device, display size and whether `com.supercell.clashofclans` is installed.

## 4. Files and updates

User configuration, statistics, attack history and diagnostics are stored under
`%APPDATA%\ClashGO`. The normal Windows installer installs the application per
user under `%LOCALAPPDATA%\Programs\TrashrProject\ClashGO Windows` so
verified in-place updates can be applied without administrator rights.

Do not delete the bundled `assets`, OpenCV DLLs or `resources` directory next
to `ClashGO.exe`; they are runtime dependencies.

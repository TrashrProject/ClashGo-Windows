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

param(
    [string]$Instance = $env:CLASHGO_BLUESTACKS_INSTANCE
)

$ErrorActionPreference = "SilentlyContinue"

if (-not $Instance) {
    $savedConfig = Join-Path $env:APPDATA "ClashGO\config.json"
    if (Test-Path $savedConfig) {
        try {
            $saved = Get-Content $savedConfig -Raw | ConvertFrom-Json
            if ($saved.device.bluestacks_instance) {
                $Instance = [string]$saved.device.bluestacks_instance
            }
        } catch {}
    }
}

function Write-Step([string]$Name, [bool]$Ok, [string]$Detail) {
    $mark = if ($Ok) { "[OK]" } else { "[!!]" }
    Write-Host ("{0} {1,-22} {2}" -f $mark, $Name, $Detail)
}

Write-Host ""
Write-Host "ClashGO Windows Doctor"
Write-Host "======================"
Write-Host ""

$playerCandidates = @()
if ($env:CLASHGO_BLUESTACKS_PLAYER) { $playerCandidates += $env:CLASHGO_BLUESTACKS_PLAYER }
if ($env:CLASHGO_BLUESTACKS_HOME) { $playerCandidates += (Join-Path $env:CLASHGO_BLUESTACKS_HOME "HD-Player.exe") }
if ($env:ProgramFiles) {
    $playerCandidates += (Join-Path $env:ProgramFiles "BlueStacks_nxt\HD-Player.exe")
    $playerCandidates += (Join-Path $env:ProgramFiles "BlueStacks\HD-Player.exe")
}
if (${env:ProgramFiles(x86)}) {
    $playerCandidates += (Join-Path ${env:ProgramFiles(x86)} "BlueStacks_nxt\HD-Player.exe")
}
$player = $playerCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
Write-Step "HD-Player.exe" ([bool]$player) $(if ($player) { $player } else { "not found" })

$confCandidates = @()
if ($env:CLASHGO_BLUESTACKS_CONF) { $confCandidates += $env:CLASHGO_BLUESTACKS_CONF }
if ($env:CLASHGO_BLUESTACKS_DATA) { $confCandidates += (Join-Path $env:CLASHGO_BLUESTACKS_DATA "bluestacks.conf") }
if ($env:ProgramData) {
    $confCandidates += (Join-Path $env:ProgramData "BlueStacks_nxt\bluestacks.conf")
    $confCandidates += (Join-Path $env:ProgramData "BlueStacks\bluestacks.conf")
}

foreach ($regPath in @("HKLM:\SOFTWARE\BlueStacks_nxt", "HKLM:\SOFTWARE\BlueStacks_msi5", "HKLM:\SOFTWARE\WOW6432Node\BlueStacks_nxt")) {
    $dataDir = (Get-ItemProperty -Path $regPath -Name DataDir -ErrorAction SilentlyContinue).DataDir
    if ($dataDir) { $confCandidates += (Join-Path $dataDir "bluestacks.conf") }
}
$conf = $confCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
Write-Step "bluestacks.conf" ([bool]$conf) $(if ($conf) { $conf } else { "not found" })

$ports = @()
if ($conf) {
    Get-Content $conf | ForEach-Object {
        if ($_ -match '^bst\.instance\.([^.]+)\.(?:status\.)?adb_port="?([0-9]+)"?$') {
            $name = $Matches[1]
            $port = [int]$Matches[2]
            $ports += [PSCustomObject]@{ Name = $name; Port = $port }
        }
    }
}
if ($ports.Count -gt 0) {
    Write-Step "Instances" $true (($ports | ForEach-Object { "$($_.Name):$($_.Port)" }) -join ", ")
} else {
    Write-Step "Instances" $false "no ADB ports found in config"
}

if ($Instance) {
    $selected = $ports | Where-Object { $_.Name -ieq $Instance } | Select-Object -First 1
    Write-Step "Selected instance" ([bool]$selected) $(if ($selected) { "$($selected.Name):$($selected.Port)" } else { "$Instance not found in config" })
}

$adbPath = $null
if ($env:CLASHGO_ADB_PATH -and (Test-Path $env:CLASHGO_ADB_PATH)) {
    $adbPath = $env:CLASHGO_ADB_PATH
}
if (-not $adbPath) {
    $adbCmd = Get-Command adb.exe -ErrorAction SilentlyContinue
    if ($adbCmd) { $adbPath = $adbCmd.Source }
}
if (-not $adbPath) {
    $adbCandidates = @()
    if ($env:CLASHGO_BLUESTACKS_HOME) { $adbCandidates += (Join-Path $env:CLASHGO_BLUESTACKS_HOME "HD-Adb.exe") }
    if ($env:ProgramFiles) {
        $adbCandidates += (Join-Path $env:ProgramFiles "BlueStacks_nxt\HD-Adb.exe")
        $adbCandidates += (Join-Path $env:ProgramFiles "BlueStacks_nxt\adb.exe")
    }
    $adbPath = $adbCandidates | Where-Object { Test-Path $_ } | Select-Object -First 1
}
Write-Step "ADB executable" ([bool]$adbPath) $(if ($adbPath) { $adbPath } else { "adb/HD-Adb.exe not found" })

if ($adbPath) {
    & $adbPath start-server | Out-Null
    foreach ($entry in $ports) {
        $addr = "127.0.0.1:$($entry.Port)"
        & $adbPath connect $addr | Out-Null
    }
    $devices = (& $adbPath devices -l) -join " | "
    Write-Step "ADB devices" ($devices -match '\bdevice\b') $devices

    $target = $null
    $probePorts = @($ports)
    if ($Instance) {
        $preferred = @($ports | Where-Object { $_.Name -ieq $Instance })
        $others = @($ports | Where-Object { $_.Name -ine $Instance })
        $probePorts = @($preferred + $others)
    }
    foreach ($entry in $probePorts) {
        $addr = "127.0.0.1:$($entry.Port)"
        $state = (& $adbPath -s $addr get-state 2>$null)
        if ($state -eq "device") {
            $target = $addr
            if (-not $Instance -or $entry.Name -ieq $Instance) { break }
        }
    }

    if ($target) {
        Write-Step "BlueStacks target" $true $target
        $manufacturer = (& $adbPath -s $target shell getprop ro.product.manufacturer 2>$null).Trim()
        $model = (& $adbPath -s $target shell getprop ro.product.model 2>$null).Trim()
        Write-Step "Android identity" $true "$manufacturer / $model"
        $size = (& $adbPath -s $target shell wm size 2>$null) -join " "
        Write-Step "Display size" ($size.Length -gt 0) $size
        $pkg = (& $adbPath -s $target shell pm path com.supercell.clashofclans 2>$null) -join " "
        Write-Step "Clash of Clans" ($pkg -match "package:") $(if ($pkg) { $pkg } else { "package not found" })
    } else {
        Write-Step "BlueStacks target" $false "no configured ADB port is online"
    }
}

Write-Host ""
Write-Host "Environment overrides supported:"
Write-Host "  CLASHGO_ADB_PATH"
Write-Host "  CLASHGO_BLUESTACKS_PLAYER"
Write-Host "  CLASHGO_BLUESTACKS_HOME"
Write-Host "  CLASHGO_BLUESTACKS_DATA"
Write-Host "  CLASHGO_BLUESTACKS_CONF"
Write-Host "  CLASHGO_BLUESTACKS_INSTANCE"
Write-Host ""

"""Real Android Debug Bridge I/O, deliberately narrow and fail-closed."""
from __future__ import annotations

import re
import struct
import subprocess
from dataclasses import dataclass

_SERIAL = re.compile(r"^[A-Za-z0-9._:-]{1,80}$")
_PNG = b"\x89PNG\r\n\x1a\n"


class AdbError(RuntimeError):
    pass


@dataclass(frozen=True)
class Screen:
    png: bytes
    width: int
    height: int


def parse_png(data: bytes) -> Screen:
    """Decode only the PNG header; image decoding belongs to the vision layer."""
    if (not isinstance(data, bytes) or len(data) < 33
            or len(data) > 16 * 1024 * 1024 or not data.startswith(_PNG)
            or data[12:16] != b"IHDR"):
        raise AdbError("Capture Android invalide ou trop volumineuse.")
    width, height = struct.unpack(">II", data[16:24])
    if not 1 <= width <= 8192 or not 1 <= height <= 8192:
        raise AdbError("Dimensions de capture non conformes.")
    return Screen(data, width, height)


class AdbClient:
    def __init__(self, serial: str, *, adb_binary="adb", timeout=20):
        if not isinstance(serial, str) or not _SERIAL.fullmatch(serial):
            raise ValueError("Choisir explicitement un serial Android valide.")
        if not isinstance(adb_binary, str) or not adb_binary.strip():
            raise ValueError("Chemin ADB invalide.")
        if not isinstance(timeout, (int, float)) or not 1 <= timeout <= 60:
            raise ValueError("Delai ADB invalide.")
        self.serial = serial
        self.adb_binary = adb_binary
        self.timeout = timeout

    @staticmethod
    def _run(args, timeout):
        try:
            result = subprocess.run(args, capture_output=True, timeout=timeout,
                                    check=False, shell=False)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise AdbError("ADB indisponible ou expiration du delai.") from exc
        if result.returncode != 0:
            # Never echo raw ADB stderr: it can contain local file paths/data.
            raise AdbError("Commande Android refusee ou appareil hors ligne.")
        return result.stdout

    @classmethod
    def devices(cls, *, adb_binary="adb"):
        raw = cls._run([adb_binary, "devices"], timeout=10).decode("utf-8", errors="replace")
        ready = []
        for line in raw.splitlines()[1:]:
            fields = line.split()
            if len(fields) >= 2 and fields[1] == "device" and _SERIAL.fullmatch(fields[0]):
                ready.append(fields[0])
        return ready

    def _device(self, *args):
        return self._run([self.adb_binary, "-s", self.serial, *args], self.timeout)

    def screenshot(self):
        return parse_png(self._device("exec-out", "screencap", "-p"))

    def _xy(self, point, screen):
        if (not isinstance(point, (list, tuple)) or len(point) != 2
                or any(type(v) not in (int, float) or not 0 <= v <= 1 for v in point)):
            raise ValueError("Coordonnees normalisees attendues entre 0 et 1.")
        return (min(screen.width - 1, round(point[0] * (screen.width - 1))),
                min(screen.height - 1, round(point[1] * (screen.height - 1))))

    def tap(self, x, y, screen):
        px, py = self._xy((x, y), screen)
        self._device("shell", "input", "tap", str(px), str(py))

    def swipe(self, x1, y1, x2, y2, duration_ms, screen):
        if type(duration_ms) is not int or not 100 <= duration_ms <= 1500:
            raise ValueError("Duree du glissement non autorisee.")
        ax, ay = self._xy((x1, y1), screen)
        bx, by = self._xy((x2, y2), screen)
        self._device("shell", "input", "swipe", str(ax), str(ay),
                     str(bx), str(by), str(duration_ms))

    def back(self):
        self._device("shell", "input", "keyevent", "KEYCODE_BACK")

    def game_running(self, package="com.supercell.clashofclans"):
        """Read-only process check; no login, launch or package installation."""
        if package != "com.supercell.clashofclans":
            raise ValueError("Package non autorise.")
        try:
            raw = self._device("shell", "pidof", package).strip()
        except AdbError:
            return False
        return bool(raw) and all(chunk.isdigit() for chunk in raw.split())

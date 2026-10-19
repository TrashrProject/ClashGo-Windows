"""Read-only first-device preflight tests: no emulator or accounts needed."""
from __future__ import annotations

import io
import unittest
from contextlib import redirect_stdout, redirect_stderr
from unittest.mock import patch

from reqngo.android.adb import AdbError, Screen
from reqngo.android.cli import main


class FakeADB:
    connected = ["emulator-5554"]
    running = True
    focused = True
    changing_size = False
    calls = []

    def __init__(self, serial, *, adb_binary="adb"):
        self.serial = serial
        self.adb_binary = adb_binary

    @classmethod
    def devices(cls, *, adb_binary="adb"):
        cls.calls.append(("devices", adb_binary))
        return list(cls.connected)

    def game_running(self):
        self.calls.append(("pidof", self.serial))
        return self.running

    def game_foreground(self):
        self.calls.append(("focus", self.serial))
        return self.focused

    def screenshot(self):
        self.calls.append(("screencap", self.serial))
        count = sum(c[0] == "screencap" for c in self.calls)
        width = 1080 if self.changing_size and count == 2 else 720
        return Screen(b"", width, 1280)


class PreflightCliTests(unittest.TestCase):
    def setUp(self):
        FakeADB.connected = ["emulator-5554"]
        FakeADB.running = True
        FakeADB.focused = True
        FakeADB.changing_size = False
        FakeADB.calls = []

    def run_cli(self, *arguments):
        output = io.StringIO()
        with patch("reqngo.android.cli.AdbClient", FakeADB):
            with redirect_stdout(output), redirect_stderr(output):
                status = main(list(arguments))
        return status, output.getvalue()

    def test_preflight_reads_twice_without_touching(self):
        status, output = self.run_cli("preflight", "--serial", "emulator-5554")
        self.assertEqual(status, 0)
        self.assertIn("PREFLIGHT OK", output)
        self.assertEqual(sum(c[0] == "screencap" for c in FakeADB.calls), 2)
        self.assertEqual([c[0] for c in FakeADB.calls],
                         ["devices", "pidof", "focus", "screencap", "screencap"])

    def test_preflight_refuses_disconnected_selected_serial(self):
        with patch("reqngo.android.cli.AdbClient", FakeADB):
            with self.assertRaisesRegex(AdbError, "non connecte"):
                main(["preflight", "--serial", "emulator-5556"])
        self.assertEqual([c[0] for c in FakeADB.calls], ["devices"])

    def test_preflight_refuses_background_or_stopped_game(self):
        for field in ("focused", "running"):
            with self.subTest(field=field):
                FakeADB.calls = []
                setattr(FakeADB, field, False)
                with patch("reqngo.android.cli.AdbClient", FakeADB):
                    with self.assertRaises(AdbError):
                        main(["preflight", "--serial", "emulator-5554"])
                self.assertNotIn("screencap", [c[0] for c in FakeADB.calls])
                setattr(FakeADB, field, True)

    def test_preflight_refuses_resolution_change(self):
        FakeADB.changing_size = True
        with patch("reqngo.android.cli.AdbClient", FakeADB):
            with self.assertRaisesRegex(AdbError, "instable"):
                main(["preflight", "--serial", "emulator-5554"])
        self.assertEqual(sum(c[0] == "screencap" for c in FakeADB.calls), 2)

    def test_no_devices_returns_nonzero_without_guessing_serial(self):
        FakeADB.connected = []
        status, output = self.run_cli("devices")
        self.assertEqual(status, 2)
        self.assertIn("Aucun appareil", output)


if __name__ == "__main__":
    unittest.main()

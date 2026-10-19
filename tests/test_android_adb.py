"""Tests for the real Android adapter using an isolated fake adb subprocess."""
import struct
import unittest
import zlib
from unittest.mock import Mock, patch

from reqngo.android.adb import AdbClient, AdbError, parse_png


def png_1x1():
    def chunk(kind, data):
        return (struct.pack(">I", len(data)) + kind + data +
                struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF))
    return (b"\x89PNG\r\n\x1a\n" +
            chunk(b"IHDR", struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0)) +
            chunk(b"IDAT", zlib.compress(b"\x00\x00\x00\x00")) +
            chunk(b"IEND", b""))


class AndroidBridgeTests(unittest.TestCase):
    def test_png_header_checked_without_image_upload(self):
        frame = parse_png(png_1x1())
        self.assertEqual((frame.width, frame.height), (1, 1))
        with self.assertRaises(AdbError):
            parse_png(b"INVALID")
        with self.assertRaises(AdbError):
            parse_png(png_1x1() + b"x" * (17 * 1024 * 1024))

    def test_device_discovery_omits_offline_and_unauthorized(self):
        output = b"List of devices attached\nemulator-5554\tdevice\nfoo\toffline\nbar\tunauthorized\n"
        with patch("subprocess.run", return_value=Mock(returncode=0, stdout=output)) as run:
            self.assertEqual(AdbClient.devices(), ["emulator-5554"])
            self.assertIs(run.call_args.kwargs["shell"], False)

    def test_screenshot_calls_only_selected_serial(self):
        with patch("subprocess.run", return_value=Mock(returncode=0, stdout=png_1x1())) as run:
            bridge = AdbClient("emulator-5554")
            self.assertEqual(bridge.screenshot().width, 1)
            self.assertEqual(run.call_args.args[0],
                             ["adb", "-s", "emulator-5554", "exec-out", "screencap", "-p"])
            self.assertFalse(run.call_args.kwargs["shell"])

    def test_normalized_tap_and_swipe_are_bounded(self):
        from reqngo.android.adb import Screen
        bridge = AdbClient("127.0.0.1:5555")
        screen = Screen(b"", 101, 201)
        with patch("subprocess.run", return_value=Mock(returncode=0, stdout=b"")) as run:
            bridge.tap(0.5, 0.25, screen)
            self.assertEqual(run.call_args.args[0][-3:], ["tap", "50", "50"])
            bridge.swipe(0, 0, 1, 1, 350, screen)
            self.assertEqual(run.call_args.args[0][-6:],
                             ["swipe", "0", "0", "100", "200", "350"])
        with self.assertRaises(ValueError):
            bridge.tap(1.1, 0.3, screen)
        with self.assertRaises(ValueError):
            bridge.swipe(0, 0, 1, 1, 20, screen)

    def test_foreground_check_refuses_other_app_and_uncertain_window(self):
        bridge = AdbClient("emulator-5554")
        results = [
            (b"mCurrentFocus=Window{u0 com.supercell.clashofclans/.Game}\n", True),
            (b"mCurrentFocus=Window{u0 com.android.settings/.Settings}\n", False),
            (b"mResumedActivity: com.supercell.clashofclans/.Game\n", False),
        ]
        for output, expected in results:
            with self.subTest(output=output), patch(
                    "subprocess.run", return_value=Mock(returncode=0, stdout=output)):
                self.assertEqual(bridge.game_foreground(), expected)

    def test_foreground_falls_back_to_displays_when_windows_lacks_field(self):
        bridge = AdbClient("emulator-5554")
        outputs = [
            Mock(returncode=0, stdout=b"Window dump without focus\n"),
            Mock(returncode=0, stdout=b"mCurrentFocus=Window{u0 com.supercell.clashofclans/.Game}\n"),
        ]
        with patch("subprocess.run", side_effect=outputs) as run:
            self.assertTrue(bridge.game_foreground())
            self.assertEqual(run.call_count, 2)
            self.assertEqual(run.call_args.args[0][-2:], ["window", "displays"])

    def test_foreground_never_overrides_other_focused_window(self):
        bridge = AdbClient("emulator-5554")
        output = b"mFocusedApp=com.supercell.clashofclans/.Game\nmCurrentFocus=Window{u0 com.android.systemui/.Dialog}\n"
        with patch("subprocess.run", return_value=Mock(returncode=0, stdout=output)) as run:
            self.assertFalse(bridge.game_foreground())
            self.assertEqual(run.call_count, 1)

    def test_foreground_full_window_fallback_rejects_stale_focused_app(self):
        bridge = AdbClient("emulator-5554")
        outputs = [
            Mock(returncode=0, stdout=b"no current focus\n"),
            Mock(returncode=0, stdout=b"mFocusedApp=com.supercell.clashofclans/.Game\n"),
            Mock(returncode=0, stdout=b"mCurrentFocus=Window{u0 com.android.settings/.Settings}\n"),
        ]
        with patch("subprocess.run", side_effect=outputs) as run:
            self.assertFalse(bridge.game_foreground())
            self.assertEqual(run.call_count, 3)
            self.assertEqual(run.call_args.args[0][-1:], ["window"])

    def test_reject_unexpected_device_serial(self):
        for serial in ("", "a b", "$(touch x)", "device;rm -rf /", "x" * 100):
            with self.assertRaises(ValueError):
                AdbClient(serial)

    def test_no_raw_sensitive_adb_stderr_in_errors(self):
        result = Mock(returncode=1, stdout=b"", stderr=b"SECRET_PRIVATE_PATH")
        with patch("subprocess.run", return_value=result):
            with self.assertRaises(AdbError) as caught:
                AdbClient("emulator-5554").screenshot()
            self.assertNotIn("SECRET_PRIVATE_PATH", str(caught.exception))


if __name__ == "__main__":
    unittest.main()

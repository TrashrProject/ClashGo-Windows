"""Opt-in, READ-ONLY Android readiness checks for explicitly selected devices.

A profile-to-ADB-serial binding is LOCAL to its authorized worker. The central
server never stores device serials, account credentials, or screenshots.
This module issues ONLY 'adb devices' and (if opted in) 'pidof' probes.
"""
from __future__ import annotations

import json
import threading

from reqngo.android.adb import AdbClient, AdbError, _SERIAL
from reqngo.fleet import _KEY


class AndroidReadOnlyPreflight:
    def __init__(self, bindings: dict[str, str], *,
                 adb_binary: str = "adb", require_game: bool = False,
                 client_class=AdbClient):
        if (not isinstance(bindings, dict) or len(bindings) > 10000
                or any(not isinstance(k, str) or not _KEY.fullmatch(k)
                       or not isinstance(v, str) or not _SERIAL.fullmatch(v)
                       for k, v in bindings.items())):
            raise ValueError("Mappage profil Android invalide.")
        if not isinstance(adb_binary, str) or not adb_binary.strip():
            raise ValueError("Chemin ADB invalide.")
        if type(require_game) is not bool:
            raise ValueError("Mode Android invalide.")
        self.bindings = dict(bindings)
        self.adb_binary = adb_binary
        self.require_game = require_game
        self.client_class = client_class
        self.serial_locks = {serial: threading.Lock() for serial in set(bindings.values())}

    @classmethod
    def from_json(cls, raw: str, **kwargs):
        if not isinstance(raw, str) or len(raw) > 1_000_000:
            raise ValueError("Configuration Android trop volumineuse.")
        try:
            value = json.loads(raw) if raw else {}
        except (ValueError, TypeError) as exc:
            raise ValueError("Configuration Android JSON invalide.") from exc
        return cls(value, **kwargs)

    def check(self, profile_key: str) -> bool:
        serial = self.bindings.get(profile_key)
        if serial is None:
            # An unbound logical profile must never be treated as healthy.
            return False
        with self.serial_locks[serial]:
            # ADB selects a real, online device; no auto-selection.
            available = self.client_class.devices(adb_binary=self.adb_binary)
            if serial not in available:
                return False
            if not self.require_game:
                return True
            client = self.client_class(serial, adb_binary=self.adb_binary, timeout=15)
            # Process presence is a read-only signal, not gameplay success.
            return bool(client.game_running())

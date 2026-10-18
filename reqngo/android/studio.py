"""Local Android Studio: real ADB screenshots, calibration and one verified action.

This is a standalone loopback-only tool, deliberately isolated from Guardian's
public API, PostgreSQL and fleet workers. It controls exactly one chosen device.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import secrets
import threading
import time
import webbrowser
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

from reqngo.android.adb import AdbClient, AdbError
from reqngo.android.runner import ActionRunner, ExecutionBlocked, Journal, Workflow
from reqngo.android.vision import TemplateVision, VisionError

_NAME = re.compile(r"[a-z][a-z0-9_]{1,39}\Z")
_WRITE_TOKEN = "I_ACCEPT_LOCAL_TOUCHES"
_MAX_BODY = 8192


class StudioError(ValueError):
    pass


def _atomic_json(path, data):
    """Replace only the local studio's JSON; no user-supplied output paths."""
    temporary = path.with_suffix(".tmp")
    try:
        with temporary.open("x", encoding="utf-8") as stream:
            json.dump(data, stream, ensure_ascii=False, indent=2)
        os.replace(temporary, path)
    finally:
        temporary.unlink(missing_ok=True)


class AndroidStudio:
    def __init__(self, serial, workspace, *, adb_binary="adb", client_class=AdbClient,
                 actions_enabled=False, clock=None):
        self.device = client_class(serial, adb_binary=adb_binary)
        self.serial = serial
        # Each physical Android serial has an independent calibration/journal.
        # A digest avoids writing the raw device serial into folder names.
        device_key = hashlib.sha256(serial.encode("utf-8")).hexdigest()[:16]
        self.folder = Path(workspace).resolve() / ("device-" + device_key)
        self.folder.mkdir(parents=True, exist_ok=True)
        self.manifest = self.folder / "states.json"
        self.workflow = self.folder / "workflow.json"
        self.journal_path = self.folder / "execution.sqlite3"
        self.lock = threading.RLock()
        # Only one route or single gesture can own the Android device at a time.
        self._route_guard = threading.Lock()
        self._route_stop = threading.Event()
        self.route_active = False
        self.clock = clock or time.monotonic
        self.actions_enabled = actions_enabled
        self.last_frame = None
        self.last_id = None
        self.frame_time = 0
        self._states = self._read(self.manifest, {"states": []}, "states")
        self._workflow = self._read(self.workflow, {"steps": []}, "steps")
        # Validate any previously created configuration before operating.
        if self._states["states"]:
            TemplateVision(self.manifest)
        if self._workflow["steps"]:
            Workflow(self.workflow, {row["name"] for row in self._states["states"]})

    @staticmethod
    def _read(path, default, key):
        if not path.exists():
            return default
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, ValueError) as exc:
            raise StudioError("Configuration locale invalide.") from exc
        if not isinstance(data, dict) or not isinstance(data.get(key), list):
            raise StudioError("Configuration locale invalide.")
        return data

    def status(self):
        with self.lock:
            journal = Journal(self.journal_path)
            try:
                index, phase = journal.state()
            finally:
                journal.close()
            return {"serial": self.serial,
                    "ready": self.device.game_running() and self.device.game_foreground(),
                    "states": [row["name"] for row in self._states["states"]],
                    "steps": self._workflow["steps"],
                    "next_step": index, "phase": phase,
                    "actions_enabled": self.actions_enabled,
                    "route_active": self.route_active}

    def capture(self):
        with self.lock:
            if not self.device.game_running() or not self.device.game_foreground():
                raise StudioError("Ouvre Clash of Clans au premier plan de l'appareil.")
            screen = self.device.screenshot()
            self.last_frame = screen
            self.last_id = secrets.token_hex(16)
            self.frame_time = self.clock()
            return self.last_id, screen

    @staticmethod
    def _coords(values, *, min_difference=0.0):
        if (not isinstance(values, list) or len(values) != 4
                or any(type(item) not in (int, float) or not 0 <= item <= 1
                       for item in values)
                or values[2] - values[0] <= min_difference
                or values[3] - values[1] <= min_difference):
            raise StudioError("Sélection incorrecte : dessine un rectangle sur l'image.")
        return values

    def save_state(self, payload):
        name = payload.get("name")
        rect = payload.get("rect")
        frame_id = payload.get("frame_id")
        if not isinstance(name, str) or not _NAME.fullmatch(name):
            raise StudioError("Nom du repère invalide (minuscules et chiffres).")
        rect = self._coords(rect)
        with self.lock:
            if (frame_id != self.last_id or not self.last_frame
                    or self.clock() - self.frame_time > 90):
                raise StudioError("Capture expirée : actualise l'écran avant d'enregistrer.")
            if not self.device.game_foreground():
                raise StudioError("Le jeu a quitté le premier plan.")
            if len(self._states["states"]) >= 30 or any(
                    item["name"] == name for item in self._states["states"]):
                raise StudioError("Nom déjà utilisé ou limite de 30 repères atteinte.")
            screen = self.last_frame
            import cv2
            import numpy as np
            original = cv2.imdecode(np.frombuffer(screen.png, dtype=np.uint8),
                                    cv2.IMREAD_GRAYSCALE)
            if original is None or original.shape != (screen.height, screen.width):
                raise StudioError("Capture PNG invalide.")
            left = int(rect[0] * screen.width)
            top = int(rect[1] * screen.height)
            right = int(rect[2] * screen.width)
            bottom = int(rect[3] * screen.height)
            if right-left < 20 or bottom-top < 20 or right > screen.width or bottom > screen.height:
                raise StudioError("Choisir une zone d'au moins 20 x 20 pixels.")
            cropped = original[top:bottom, left:right]
            if float(cropped.std()) < 8:
                raise StudioError("Repère trop uniforme : choisir un élément distinctif.")
            ok, png = cv2.imencode(".png", cropped)
            if not ok or len(png) > 5_000_000:
                raise StudioError("Repère PNG impossible à enregistrer.")
            path = self.folder / (name + ".png")
            # No overwrite of an existing private capture.
            with path.open("xb") as stream:
                stream.write(png.tobytes())
            # Search close to the original position, with a slight margin.
            region = [max(0., rect[0]-.05), max(0., rect[1]-.05),
                      min(1., rect[2]+.05), min(1., rect[3]+.05)]
            new_config = {"states": [*self._states["states"],
                                     {"name": name, "template": name+".png",
                                      "threshold": 0.92, "region": region}]}
            try:
                _atomic_json(self.manifest, new_config)
                TemplateVision(self.manifest)
            except BaseException:
                path.unlink(missing_ok=True)
                raise
            self._states = new_config
            return {"state": name, "resolution": [screen.width, screen.height]}

    def save_step(self, payload):
        name, before, after = (payload.get(k) for k in ("name", "before", "after"))
        position = payload.get("position")
        if not isinstance(name, str) or not _NAME.fullmatch(name):
            raise StudioError("Nom d'étape invalide.")
        if (not isinstance(position, list) or len(position) != 2
                or any(type(c) not in (int, float) or not 0 <= c <= 1
                       for c in position)):
            raise StudioError("Sélectionne un point de clic valide.")
        with self.lock:
            known = {row["name"] for row in self._states["states"]}
            if (before not in known or after not in known or before == after):
                raise StudioError("Deux états visuels distincts sont requis.")
            if len(self._workflow["steps"]) >= 30 or any(
                    step["name"] == name for step in self._workflow["steps"]):
                raise StudioError("Nom d'étape dupliqué ou limite atteinte.")
            step = {"name": name, "before": before, "after": after,
                    "action": "tap", "points": position, "timeout_seconds": 15}
            result = {"steps": [*self._workflow["steps"], step]}
            # Check a new workflow before replacing the active file.
            candidate = self.folder / "workflow-candidate.json"
            _atomic_json(candidate, result)
            try:
                Workflow(candidate, known)
                os.replace(candidate, self.workflow)
            finally:
                candidate.unlink(missing_ok=True)
            self._workflow = result
            return {"step": name, "index": len(result["steps"])-1}

    def _build_runner(self, journal):
        if not self._workflow["steps"] or not self._states["states"]:
            raise StudioError("Calibre deux etats puis une etape avant de demarrer.")
        vision = TemplateVision(self.manifest)
        workflow = Workflow(self.workflow, vision.states)
        return ActionRunner(self.device, vision, workflow, journal)

    def run_once(self, *, execute=False, confirmation=""):
        if not self._route_guard.acquire(blocking=False):
            raise StudioError("Un parcours est deja en cours sur cet appareil.")
        try:
            with self.lock:
                if execute and (not self.actions_enabled
                                or confirmation != "EXECUTER UNE ACTION"):
                    raise StudioError("Geste non autorise : confirme l'operation localement.")
                journal = Journal(self.journal_path)
                try:
                    return self._build_runner(journal).step(dry_run=not execute)
                finally:
                    journal.close()
        finally:
            self._route_guard.release()

    def stop_route(self):
        """Interrupt a route BETWEEN gestures; never interrupt ADB mid-tap."""
        active = self.route_active
        if active:
            self._route_stop.set()
        return {"stopping": active}

    def run_route(self, *, confirmation="", max_steps=10):
        """Bounded, opt-in, visual-confirmation-driven sequence.

        No fixed account-switch delay: after each postcondition is confirmed,
        attempt the next calibrated step immediately. Never loop back to step 0.
        The stop flag is intentionally independent of the studio lock.
        """
        if (not self.actions_enabled or confirmation != "EXECUTER LE PARCOURS"):
            raise StudioError("Confirme explicitement le parcours rapide local.")
        if type(max_steps) is not int or not 1 <= max_steps <= 10:
            raise StudioError("Le parcours est limite a 1-10 gestes verifies.")
        if not self._route_guard.acquire(blocking=False):
            raise StudioError("Une autre operation Android est deja en cours.")
        try:
            with self.lock:
                self.route_active = True
                self._route_stop.clear()
                started = time.monotonic()
                completed = 0
                step_timings = []
                reason = "limit"
                journal = Journal(self.journal_path)
                try:
                    runner = self._build_runner(journal)
                    if journal.state()[1] != "ready":
                        raise StudioError(
                            "Journal non pret : verifier manuellement l'action interrompue.")
                    while completed < max_steps:
                        if self._route_stop.is_set():
                            reason = "stopped"
                            break
                        if time.monotonic() - started >= 90:
                            reason = "time_limit"
                            break
                        step_name = runner.workflow.steps[journal.state()[0]].name
                        step_started = time.monotonic()
                        result = runner.step(dry_run=False)
                        if result["status"] == "complete":
                            reason = "complete"
                            break
                        if result["status"] != "verified":
                            # Ambiguous restart/recovery requires a separate check.
                            reason = "recovered"
                            break
                        completed += 1
                        step_timings.append({
                            "step": step_name,
                            "elapsed_ms": round((time.monotonic() - step_started) * 1000)
                        })
                        if journal.state()[0] >= len(runner.workflow.steps):
                            reason = "complete"
                            break
                    return {
                        "status": "route_finished",
                        "reason": reason,
                        "verified_steps": completed,
                        "step_timings": step_timings,
                        "elapsed_ms": round((time.monotonic() - started) * 1000),
                        "next_step": journal.state()[0],
                    }
                finally:
                    journal.close()
                    self.route_active = False
                    self._route_stop.clear()
        finally:
            self._route_guard.release()


class StudioHTTPServer(ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True

    def __init__(self, address, studio):
        if address[0] != "127.0.0.1":
            raise ValueError("Ce studio est réservé à la machine locale.")
        super().__init__(address, StudioHandler)
        self.studio = studio
        self.secret = secrets.token_hex(32)


class StudioHandler(BaseHTTPRequestHandler):
    server: StudioHTTPServer

    def log_message(self, format, *args):
        # Requests can contain local filesystem or profile information.
        pass

    def _headers(self, status, mime="application/json", length=None, extra=None):
        self.send_response(status)
        self.send_header("Content-Type", mime)
        self.send_header("Cache-Control", "no-store")
        self.send_header("X-Content-Type-Options", "nosniff")
        self.send_header("X-Frame-Options", "DENY")
        self.send_header("Referrer-Policy", "no-referrer")
        self.send_header("Cross-Origin-Resource-Policy", "same-origin")
        self.send_header("Content-Security-Policy",
                         "default-src 'none'; script-src 'self' 'unsafe-inline'; "
                         "style-src 'self' 'unsafe-inline'; img-src 'self' blob:; "
                         "connect-src 'self'; frame-ancestors 'none'")
        if length is not None:
            self.send_header("Content-Length", str(length))
        if extra:
            for key, value in extra.items():
                self.send_header(key, value)
        self.end_headers()

    def _json(self, data, status=200):
        payload = json.dumps(data, ensure_ascii=False).encode("utf-8")
        self._headers(status, length=len(payload))
        self.wfile.write(payload)

    def _allowed_host(self):
        return self.headers.get("Host", "") == f"127.0.0.1:{self.server.server_port}"

    def _error(self, message, status=422):
        self._json({"error": message}, status)

    def do_GET(self):
        if not self._allowed_host():
            return self._error("Hôte non autorisé.", 403)
        route = urlsplit(self.path).path
        try:
            if route == "/":
                html = (Path(__file__).with_name("studio.html")
                        .read_text(encoding="utf-8")
                        .replace("__REQNGO_LOCAL_TOKEN__", self.server.secret))
                data = html.encode("utf-8")
                self._headers(200, "text/html; charset=utf-8", len(data))
                self.wfile.write(data)
            elif route == "/api/status":
                self._json(self.server.studio.status())
            elif route == "/api/frame":
                frame_id, screen = self.server.studio.capture()
                self._headers(200, "image/png", len(screen.png),
                              {"X-Frame-Id": frame_id})
                self.wfile.write(screen.png)
            else:
                self._error("Page introuvable.", 404)
        except (AdbError, StudioError, VisionError, OSError, ImportError) as exc:
            self._error(str(exc))

    def do_POST(self):
        if not self._allowed_host():
            return self._error("Hôte non autorisé.", 403)
        if self.headers.get("Origin", "") != f"http://127.0.0.1:{self.server.server_port}":
            return self._error("Origine non autorisée.", 403)
        if not secrets.compare_digest(
                self.headers.get("X-REQNGO-Local-Token", ""), self.server.secret):
            return self._error("Jeton local invalide.", 403)
        length = self.headers.get("Content-Length", "")
        if not length.isdecimal() or not 1 <= int(length) <= _MAX_BODY:
            return self._error("Requête invalide ou trop grande.", 413)
        if self.headers.get("Content-Type", "").split(";")[0] != "application/json":
            return self._error("Type de requête incorrect.", 415)
        try:
            data = json.loads(self.rfile.read(int(length)))
            if not isinstance(data, dict):
                raise StudioError("Requête JSON invalide.")
            route = urlsplit(self.path).path
            if route == "/api/state":
                result = self.server.studio.save_state(data)
            elif route == "/api/step":
                result = self.server.studio.save_step(data)
            elif route == "/api/run":
                if set(data) - {"execute", "confirmation"} or type(
                        data.get("execute", False)) is not bool:
                    raise StudioError("Commande inconnue.")
                result = self.server.studio.run_once(
                    execute=data.get("execute", False),
                    confirmation=data.get("confirmation", ""))
            elif route == "/api/route":
                if set(data) - {"confirmation", "max_steps"}:
                    raise StudioError("Commande de parcours inconnue.")
                result = self.server.studio.run_route(
                    confirmation=data.get("confirmation", ""),
                    max_steps=data.get("max_steps", 10))
            elif route == "/api/route/stop":
                if data:
                    raise StudioError("La commande d'arret doit etre vide.")
                result = self.server.studio.stop_route()
            else:
                return self._error("Route non autorisée.", 404)
            self._json(result)
        except (AdbError, StudioError, VisionError, ExecutionBlocked,
                ValueError, OSError, ImportError) as exc:
            self._error(str(exc))


def main(argv=None):
    parser = argparse.ArgumentParser(description="REQ N GO — Android Studio local")
    parser.add_argument("--serial", required=True, help="Identifiant de l'appareil choisi")
    parser.add_argument("--adb", default=os.environ.get("ADB_BINARY", "adb"))
    parser.add_argument("--workspace", default="data/android-studio")
    parser.add_argument("--port", type=int, default=8766)
    parser.add_argument("--no-browser", action="store_true")
    args = parser.parse_args(argv)
    if not 1024 <= args.port <= 65535:
        parser.error("Port local invalide.")
    if args.serial not in AdbClient.devices(adb_binary=args.adb):
        parser.error("Appareil Android introuvable : vérifier adb devices.")
    studio = AndroidStudio(args.serial, args.workspace, adb_binary=args.adb,
                           actions_enabled=os.environ.get("REQNGO_ANDROID_ACTIONS")
                                           == _WRITE_TOKEN)
    server = StudioHTTPServer(("127.0.0.1", args.port), studio)
    address = f"http://127.0.0.1:{args.port}"
    print("REQ N GO Android Studio sur", address, flush=True)
    print("Appareil sélectionné :", args.serial, flush=True)
    print("Gestes tactiles autorisés :", studio.actions_enabled, flush=True)
    print("La calibration est enregistrée localement dans :", studio.folder, flush=True)
    if not args.no_browser:
        webbrowser.open(address)
    try:
        server.serve_forever(poll_interval=.3)
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()

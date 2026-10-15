"""End-to-end test: three actual independent Python pilot processes over local HTTP."""
import json
import os
import subprocess
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

from reqngo.telemetry import TelemetryStore


class ThreeProcessesTest(unittest.TestCase):
    def test_three_real_workers_report_to_one_guardian(self):
        with tempfile.TemporaryDirectory() as tmp:
            store = TelemetryStore(Path(tmp) / "guardian.sqlite3")
            key = "test-only-token-" + "x" * 40

            class Handler(BaseHTTPRequestHandler):
                def log_message(self, *args):
                    pass

                def do_POST(self):
                    if self.headers.get("Authorization") != "Bearer " + key:
                        self.send_error(401)
                        return
                    size = int(self.headers.get("Content-Length", "0"))
                    data = json.loads(self.rfile.read(size))
                    if self.path == "/api/telemetry/heartbeat":
                        result = store.heartbeat(**data)
                    elif self.path == "/api/telemetry/report":
                        result = store.report(**data)
                    else:
                        self.send_error(404)
                        return
                    body = json.dumps(result).encode()
                    self.send_response(200)
                    self.send_header("Content-Type", "application/json")
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)

            server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
            thread = threading.Thread(target=server.serve_forever, daemon=True)
            thread.start()
            processes = []
            try:
                for n in range(1, 4):
                    env = dict(os.environ)
                    env.update({
                        "GUARDIAN_URL": "http://127.0.0.1:" + str(server.server_port),
                        "GUARDIAN_AGENT_TOKEN": key,
                        "PILOT_DATA_DIR": str(Path(tmp) / ("pilot_" + str(n))),
                    })
                    processes.append(subprocess.Popen(
                        [sys.executable, "-m", "reqngo.pilot_worker",
                         "--id", "pilot_0" + str(n), "--profile", str(n), "--once"],
                        env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                        text=True,
                    ))
                for proc in processes:
                    stdout, stderr = proc.communicate(timeout=20)
                    self.assertEqual(proc.returncode, 0, stderr + stdout)
                snapshot = store.snapshot()
                self.assertEqual(snapshot["summary"]["connected"], 3)
                self.assertEqual({a["agent_id"] for a in snapshot["agents"]},
                                 {"pilot_01", "pilot_02", "pilot_03"})
                self.assertEqual(snapshot["summary"]["open_cases"], 0)
                self.assertEqual({e["profile_id"] for e in snapshot["events"]},
                                 {1, 2, 3})
                self.assertEqual(len(snapshot["events"]), 6)
            finally:
                for proc in processes:
                    if proc.poll() is None:
                        proc.kill()
                        proc.communicate()
                server.shutdown()
                server.server_close()
                thread.join(timeout=4)
                store.close()


if __name__ == "__main__":
    unittest.main()

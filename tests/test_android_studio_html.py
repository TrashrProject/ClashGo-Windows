"""Static smoke checks for the *functional* localhost Android cockpit UI."""
import re
import shutil
import subprocess
import tempfile
import unittest
from html.parser import HTMLParser
from pathlib import Path

PAGE = Path(__file__).resolve().parents[1] / "reqngo" / "android" / "studio.html"


class Parser(HTMLParser):
    def __init__(self):
        super().__init__()
        self.ids = []
        self.inside = False
        self.scripts = []

    def handle_starttag(self, name, attributes):
        fields = dict(attributes)
        if fields.get("id"):
            self.ids.append(fields["id"])
        if name == "script" and "src" not in fields:
            self.inside = True

    def handle_endtag(self, name):
        if name == "script":
            self.inside = False

    def handle_data(self, value):
        if self.inside:
            self.scripts.append(value)


class AndroidStudioHTMLTests(unittest.TestCase):
    def test_dom_controls_are_real_and_connected(self):
        data = PAGE.read_text(encoding="utf-8")
        parsed = Parser()
        parsed.feed(data)
        self.assertEqual(len(parsed.ids), len(set(parsed.ids)))
        js = "".join(parsed.scripts)
        references = set(re.findall(r'el\("([^"]+)"\)', js))
        self.assertFalse(references - set(parsed.ids))
        for endpoint in ("/api/frame", "/api/state", "/api/step", "/api/run",
                         "/api/status", "/api/route", "/api/route/stop"):
            self.assertIn(endpoint, js)
        self.assertIn("X-REQNGO-Local-Token", js)
        self.assertIn("EXECUTER UNE ACTION", js)
        self.assertIn("EXECUTER LE PARCOURS", js)
        self.assertIn("fast-start", parsed.ids)
        self.assertIn("fast-stop", parsed.ids)
        self.assertIn("setPointerCapture", js)
        self.assertNotIn("innerHTML", js)
        self.assertNotIn("eval(", js)

    @unittest.skipUnless(shutil.which("node"), "Node.js not installed")
    def test_studio_script_syntax(self):
        parsed = Parser()
        parsed.feed(PAGE.read_text(encoding="utf-8"))
        with tempfile.TemporaryDirectory() as tmp:
            script = Path(tmp) / "studio.js"
            script.write_text("".join(parsed.scripts), encoding="utf-8")
            output = subprocess.run(["node", "--check", str(script)],
                                    text=True, capture_output=True,
                                    check=False, timeout=10)
            self.assertEqual(output.returncode, 0, output.stderr)


if __name__ == "__main__":
    unittest.main()

"""Static checks for the functioning fleet console; HTML is not a screenshot."""
import re
import shutil
import subprocess
import tempfile
import unittest
from html.parser import HTMLParser
from pathlib import Path


PAGE = Path(__file__).resolve().parents[1] / "reqngo" / "static" / "fleet.html"
LEGACY = Path(__file__).resolve().parents[1] / "reqngo" / "static" / "index.html"


class Inspect(HTMLParser):
    def __init__(self):
        super().__init__()
        self.ids = []
        self.inline_scripts = []
        self.in_script = False

    def handle_starttag(self, tag, attrs):
        attributes = dict(attrs)
        if "id" in attributes:
            self.ids.append(attributes["id"])
        if tag == "script" and "src" not in attributes:
            self.in_script = True

    def handle_endtag(self, tag):
        if tag == "script":
            self.in_script = False

    def handle_data(self, value):
        if self.in_script:
            self.inline_scripts.append(value)


class FleetMarkupTests(unittest.TestCase):
    def test_only_unique_ids_and_active_controls(self):
        html = PAGE.read_text(encoding="utf-8")
        parsed = Inspect()
        parsed.feed(html)
        self.assertEqual(len(parsed.ids), len(set(parsed.ids)))
        script = "".join(parsed.inline_scripts)
        self.assertGreater(len(script), 5000)
        references = set(re.findall(r'el\("([^"]+)"\)', script))
        self.assertFalse(references - set(parsed.ids))
        self.assertIn("/api/fleet/console/profiles", script)
        self.assertIn("/api/fleet/console/assign", script)
        self.assertIn("/api/fleet/console/groups", script)
        self.assertIn("expected_count", script)
        self.assertIn('href="/fleet"', LEGACY.read_text(encoding="utf-8"))
        self.assertNotIn("innerHTML", script)
        self.assertNotIn("eval(", script)

    @unittest.skipUnless(shutil.which("node"), "Node.js not installed")
    def test_inline_javascript_compiles(self):
        parsed = Inspect()
        parsed.feed(PAGE.read_text(encoding="utf-8"))
        with tempfile.TemporaryDirectory() as directory:
            js_file = Path(directory) / "fleet-console.js"
            js_file.write_text("".join(parsed.inline_scripts), encoding="utf-8")
            result = subprocess.run(
                ["node", "--check", str(js_file)],
                text=True, capture_output=True, check=False, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()

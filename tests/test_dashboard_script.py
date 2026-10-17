"""Prevent client-side syntax errors and missing DOM targets in the dashboard."""
import re
import shutil
import subprocess
import unittest
from pathlib import Path


class DashboardScriptTests(unittest.TestCase):
    def test_dashboard_script_targets_and_syntax(self):
        html = (Path(__file__).resolve().parents[1] /
                "reqngo/static/index.html").read_text(encoding="utf-8")
        scripts = re.findall(r"<script>(.*?)</script>", html, flags=re.DOTALL)
        self.assertEqual(len(scripts), 1)
        ids = set(re.findall(r'\bid="([^"]+)"', html))
        targets = set(re.findall(r'document\.getElementById\("([^"]+)"\)', scripts[0]))
        self.assertFalse(targets - ids, "Missing DOM elements: " + str(targets - ids))
        node = shutil.which("node")
        if node is None:
            self.skipTest("Node.js is not installed; syntax checked in GitHub CI")
        result = subprocess.run([node, "--check"], input=scripts[0],
                                text=True, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == "__main__":
    unittest.main()

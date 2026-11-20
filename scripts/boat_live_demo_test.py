"""No network or real subprocesses: protocol and whole-flow checks."""
import argparse
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import urllib.error

import boat_live_demo as demo


class FakeDemo(demo.Demo):
    def __init__(self, args, root):
        super().__init__(args, root)
        self.calls = []
        self.vms = {}
        self.snapshots = set()
        self.layer_deleted = set()

    def api(self, method, path, body=None):
        self.calls.append((method, path))
        if path.startswith("/sandboxes?"):
            return {"sandboxes": list(self.vms.values())}
        if path == "/sandboxes" and method == "POST":
            self.vms["probe"] = {"id": "probe", "state": "ready"}
            return {"sandbox": self.vms["probe"]}
        if path.endswith("/usage"):
            return {"seconds": 1, "dollars": 0.00001}
        if path.startswith("/sandboxes/"):
            name = path.split("/")[-1]
            if method == "DELETE":
                self.vms.pop(name, None)
                return {}
            return {"sandbox": self.vms[name]}
        if path == "/saved-snapshots":
            self.snapshots.add(body["name"])
            return {}
        if path.startswith("/saved-snapshots/"):
            if method == "DELETE":
                self.snapshots.discard(path.split("/")[-1])
                return {}
            return {"savedSnapshot": {"status": "ready"}}
        if path == "/environments":
            return {"environments": [{"id": "env", "name": body["name"]}]}
        if path.startswith("/environments/"):
            return {}
        raise AssertionError((method, path))

    def layer(self, method, path):
        self.calls.append((method, path))
        if method == "DELETE":
            self.layer_deleted.add(path.rsplit("/", 1)[-1])
            return {}
        raise demo.APIError(404, "not_found")

    def hev(self, *args, display=False):
        self.calls.append(tuple(args))
        if args[0] == "query":
            return '[{"text":"boat-demo-smoke"}]'
        if args[0] == "traces":
            assert "--json" not in args
            return "STARTED ID\nnow session\n"
        self.assert_dispatch(args)
        action = args[1]
        if action.endswith(".md"):
            name = Path(action).stem
            order = Path(action).read_text()
            assert "repo: hev/kit" in order
            assert "Never commit, push, open a PR" in order
            self.vms[name] = {"id": name, "state": "ready",
                              "labels": {"hev.order": name}}
            return "queued" if len(self.orders) == 4 else "started"
        if action == "show":
            return json.dumps({"files": {"result.json": json.dumps({"end": "now"})}})
        if action == "clean":
            self.vms.pop(args[2], None)
        return "{}"

    @staticmethod
    def assert_dispatch(args):
        assert args[0] == "dispatch"
        assert args[1] != "start", "CLI takes an order path, not start subcommand"


class DemoTest(unittest.TestCase):
    def test_full_flow_and_cleanup(self):
        environment = {"BOAT_API_KEY": "synthetic", "BOAT_SSH_PUBLIC_KEY": "public",
                       "HEVLAYER_API_KEY": "layer-synthetic",
                       "HEVLAYER_ENDPOINT": "https://layer.example.invalid"}
        with patch.dict(os.environ, environment), tempfile.TemporaryDirectory() as root:
            instance = FakeDemo(argparse.Namespace(template="prepared", delete_template=False), root)
            with patch.object(demo.subprocess, "run",
                              return_value=subprocess.CompletedProcess([], 0)):
                instance.run()
                instance.cleanup()
            self.assertEqual(len(instance.orders), 14)
            self.assertFalse(instance.vms)
            self.assertFalse(instance.snapshots)
            self.assertEqual(instance.layer_deleted,
                             {demo.NAMESPACE + suffix for suffix in demo.SUFFIXES})
            self.assertTrue(any(call[:2] == ("dispatch", "rework") for call in instance.calls))
            self.assertTrue(any(call[:2] == ("traces", "ls") for call in instance.calls))
            self.assertTrue(any(call[0] == "query" for call in instance.calls))

    def test_trial_rejection_is_exact_and_keys_are_not_printed(self):
        error = urllib.error.HTTPError("https://example.invalid", 403, "", {}, io.BytesIO(
            b'{"code":"trial_saved_snapshots_not_allowed","secret":"never-print"}'))
        with patch.object(demo.urllib.request, "urlopen", side_effect=error):
            with self.assertRaises(demo.APIError) as raised:
                demo.request(demo.BASE, "private-key", "POST", "/saved-snapshots", {})
        self.assertEqual(str(raised.exception), "trial_saved_snapshots_not_allowed")

    def test_arbitrary_provider_detail_is_withheld(self):
        error = urllib.error.HTTPError("https://example.invalid", 500, "", {}, io.BytesIO(
            b'{"code":"private-key","details":"never-print"}'))
        with patch.object(demo.urllib.request, "urlopen", side_effect=error):
            with self.assertRaises(demo.APIError) as raised:
                demo.request(demo.BASE, "private-key", "GET", "/sandboxes")
        self.assertNotIn("private-key", str(raised.exception))


if __name__ == "__main__":
    unittest.main()

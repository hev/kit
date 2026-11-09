"""Offline-testable paid-plan Boat demo. Never prints API bodies or keys."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

BASE = "https://boat.dev/api/v1"
NAMESPACE = "boat-demo"
SUFFIXES = ("", "-sessions", "-blocks", "-evals")


class APIError(RuntimeError):
    def __init__(self, status, code):
        self.status = status
        self.code = code
        super().__init__(code if code == "trial_saved_snapshots_not_allowed"
                         else "HTTP " + str(status) + " (provider detail withheld)")


def request(base, key, method, path, body=None):
    data = None if body is None else json.dumps(body).encode()
    headers = {"Authorization": "Bearer " + key, "Content-Type": "application/json"}
    if method == "DELETE":
        headers["X-Ascii-Confirm-Delete"] = path.rsplit("/", 1)[-1]
    req = urllib.request.Request(base + path, data=data, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=660) as response:
            raw = response.read()
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as error:
        # Only the exact trial rejection is safe to print; provider bodies
        # and arbitrary codes could contain submitted credentials.
        try:
            code = json.loads(error.read()).get("code", "")
        except (ValueError, TypeError):
            code = ""
        raise APIError(error.code, code) from None
    except urllib.error.URLError:
        raise RuntimeError("API transport failed (details withheld)") from None


def smoke_order(name, host, hold=5):
    return f"""---
name: {name}
repo: hev/kit
host: {host}
harness: codex
validation: none
timeout: 15
---
This is a read-only public-repository cloud smoke test.
Run ls and git status. Run sleep {hold} before writing report.md.
Never commit, push, open a PR, send messages, or change repository files.
Write boat-demo-smoke and the current branch to report.md, then finish.
## Acceptance
- report.md contains boat-demo-smoke.
- No repository files changed; no commit, push or PR.
"""


def wait_until(check, seconds=1200):
    deadline = time.monotonic() + seconds
    while True:
        value = check()
        if value:
            return value
        if time.monotonic() >= deadline:
            raise RuntimeError("demo acceptance timed out")
        time.sleep(3)


class Demo:
    def __init__(self, args, root):
        self.args, self.root = args, Path(root)
        self.token = uuid.uuid4().hex[:10]
        self.host = "boat-demo-" + self.token
        self.environment_id = None
        self.namespace_owned = False
        self.secret_names = []
        self.orders = []
        self.snapshot = args.template or "hev-demo-template-" + self.token
        self.snapshot_owned = not args.template
        self.probe_snapshot = "hev-demo-probe-" + self.token
        self.probe_id = None
        self.usage_by_id = {}
        self.key = os.environ["BOAT_API_KEY"]
        self.layer_key = os.environ.get("HEVLAYER_API_KEY") or os.environ["LAYER_API_KEY"]
        self.layer_endpoint = (os.environ.get("HEVLAYER_ENDPOINT")
                               or os.environ["LAYER_ENDPOINT"]).rstrip("/")
        self.env = dict(os.environ, HEVLAYER_NAMESPACE=NAMESPACE,
                        HEVLAYER_API_KEY=self.layer_key,
                        HEVLAYER_ENDPOINT=self.layer_endpoint)
        self.binary = str(self.root / "hev")
        self.config = self.root / "dispatch.toml"
        self.repo = Path(__file__).resolve().parent.parent

    def api(self, method, path, body=None):
        return request(BASE, self.key, method, path, body)

    def layer(self, method, path):
        return request(self.layer_endpoint, self.layer_key, method, path)

    def namespace_preflight(self):
        for suffix in SUFFIXES:
            try:
                self.layer("GET", "/v1/namespaces/" + NAMESPACE + suffix + "/schema")
            except APIError as error:
                if error.status == 404:
                    continue
                raise
            raise RuntimeError("boat-demo namespace already exists; use an empty dedicated namespace")
        self.namespace_owned = True

    def usage(self, machine_id):
        data = self.api("GET", "/sandboxes/" + machine_id + "/usage")
        self.usage_by_id[machine_id] = (int(data["seconds"]), float(data["dollars"]))

    def machines(self):
        result, cursor = [], ""
        while True:
            query = urllib.parse.urlencode({"label": "hev.dispatch=" + self.host,
                                             "limit": 100, "cursor": cursor})
            data = self.api("GET", "/sandboxes?" + query)
            result.extend(data["sandboxes"])
            page = data.get("pageInfo", {})
            if not page.get("hasMore"):
                return result
            next_cursor = page.get("nextCursor")
            if not next_cursor or next_cursor == cursor:
                raise RuntimeError("invalid provider pagination")
            cursor = next_cursor

    def saved_snapshot_probe(self):
        self.probe_id = self.api("POST", "/sandboxes", {
            "name": "hev-demo-probe-" + self.token, "type": "small", "noEnv": True,
            "ttlSeconds": 600, "labels": {"hev.dispatch": self.host}
        })["sandbox"]["id"]
        wait_until(lambda: self.api("GET", "/sandboxes/" + self.probe_id)["sandbox"]["state"]
                   in ("ready", "idle", "running"))
        self.api("POST", "/saved-snapshots",
                 {"sandboxId": self.probe_id, "name": self.probe_snapshot})
        wait_until(lambda: self.api("GET", "/saved-snapshots/" + self.probe_snapshot)
                   .get("savedSnapshot", {}).get("status") == "ready")
        self.api("DELETE", "/saved-snapshots/" + self.probe_snapshot)
        self.usage(self.probe_id)
        self.api("DELETE", "/sandboxes/" + self.probe_id)
        self.probe_id = None

    def configure(self, cap):
        self.config.write_text(
            'default_host = "' + self.host + '"\n[hosts.' + self.host + ']\n'
            'type = "boat"\ncap = ' + str(cap) + '\nfrom = ' + json.dumps(self.snapshot)
            + '\nsize = "small"\nnamespace = "boat-demo"\nenvironment = '
            + json.dumps(self.environment_id) + '\n')
        self.config.chmod(0o600)

    def hev(self, *args, display=False):
        result = subprocess.run([self.binary, *args], env=self.env,
                                text=True, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, timeout=900)
        if result.returncode:
            raise RuntimeError("hev " + args[0] + " failed (output withheld)")
        if display:
            # Smoke output should be harmless, but redact env values anyway.
            output = result.stdout
            for name, value in self.env.items():
                if value and any(word in name for word in ("KEY", "TOKEN", "SECRET")):
                    output = output.replace(value, "[redacted]")
            print(output, end="", flush=True)
        return result.stdout

    def dispatch(self, action, *args, display=False):
        return self.hev("dispatch", action, *args, "--host", self.host,
                        "--config", str(self.config), display=display)

    def start(self, name, hold):
        path = self.root / (name + ".md")
        path.write_text(smoke_order(name, self.host, hold))
        # Register before starting, so cleanup also cancels queued requests.
        self.orders.append(name)
        return self.hev("dispatch", str(path), "--host", self.host, "--config", str(self.config))

    def terminal(self, name):
        data = json.loads(self.dispatch("show", name, "--json"))
        result = json.loads(data.get("files", {}).get("result.json") or "{}")
        return bool(result.get("end")) and not result.get("launch_unconfirmed")

    def stop_clean(self, name):
        if not self.terminal(name):
            self.dispatch("stop", name)
        self.dispatch("clean", name)

    def run(self):
        self.namespace_preflight()
        subprocess.run(["go", "build", "-o", self.binary, "./cmd/hev"],
                       cwd=self.repo, check=True, stdout=subprocess.DEVNULL)
        self.saved_snapshot_probe()  # exact trial error stops before secrets
        if self.snapshot_owned:
            built = subprocess.run([str(self.repo / "scripts/boat-template.sh"), self.snapshot],
                                   cwd=self.repo, env=self.env, check=True,
                                   text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            for line in built.stdout.splitlines():
                if line.startswith("Template usage: "):
                    usage = json.loads(line.removeprefix("Template usage: "))
                    self.usage_by_id["template-build"] = (int(usage["seconds"]), float(usage["dollars"]))
        env_name = "hev-demo-" + self.token
        environments = self.api("POST", "/environments", {"name": env_name})["environments"]
        self.environment_id = next(e["id"] for e in environments if e["name"] == env_name)
        self.api("PUT", "/environments/" + self.environment_id, {
            "passSecrets": True, "passAgentsCredentials": True,
            "passGithub": False, "passSandboxCredentials": False})
        for name, value in {
            "HEVLAYER_API_KEY": self.layer_key, "HEVLAYER_NAMESPACE": NAMESPACE,
            "HEVLAYER_ENDPOINT": self.layer_endpoint,
            **({"OPENAI_API_KEY": self.env["OPENAI_API_KEY"]}
               if self.env.get("OPENAI_API_KEY") else {})
        }.items():
            # Track before mutation: an HTTP transport failure can be ambiguous.
            self.secret_names.append(name)
            self.api("PUT", "/environments/" + self.environment_id + "/vars/" + name,
                     {"value": value})
        self.configure(3)
        first = ["demo-" + self.token + "-" + str(i) for i in range(1, 5)]
        with ThreadPoolExecutor(max_workers=3) as pool:
            list(pool.map(lambda n: self.start(n, 300), first[:3]))
        self.dispatch("ls", display=True)
        self.dispatch("show", first[0], display=True)
        self.dispatch("stop", first[0])
        self.dispatch("rework", first[0], "Continue the same read-only smoke order.")
        queued = self.start(first[3], 5)
        if "queued" not in queued:
            raise RuntimeError("fourth order did not queue at cap")
        self.dispatch("stop", first[1])
        wait_until(lambda: any(m.get("labels", {}).get("hev.order") == first[3]
                               for m in self.machines()))
        for name in first:
            if name != first[1]:
                wait_until(lambda n=name: self.terminal(n))
        traces = self.hev("traces", "ls", "--order", first[0], display=True)
        if len(traces.strip().splitlines()) < 2:
            raise RuntimeError("no trace captured for smoke order")
        for machine in self.machines():
            self.usage(machine["id"])
        for name in first:
            self.dispatch("clean", name)
        self.configure(10)
        ten = ["demo-" + self.token + "-many-" + str(i) for i in range(10)]
        with ThreadPoolExecutor(max_workers=10) as pool:
            list(pool.map(lambda n: self.start(n, 5), ten))
        self.dispatch("ls", display=True)
        for name in ten:
            wait_until(lambda n=name: self.terminal(n))
        hits = json.loads(self.hev("query", "boat-demo-smoke", "--namespace", NAMESPACE, "--json", display=True))
        if not hits:
            raise RuntimeError("cross-order query returned no trace hits")
        for machine in self.machines():
            self.usage(machine["id"])
        for name in ten:
            self.dispatch("clean", name)

    def cleanup(self):
        errors = []
        def attempt(fn):
            try:
                fn()
            except Exception:
                errors.append("cleanup operation failed (details withheld)")
        if self.config.exists():
            active_names = {m.get("labels", {}).get("hev.order") for m in self.machines()}
            queue = Path.home() / "dispatch" / ".remote" / self.host / ".queue"
            for name in self.orders:
                if name in active_names or (queue / (name + ".json")).exists():
                    attempt(lambda n=name: self.dispatch("stop", n))
        # Inventory is authoritative even if a CLI returned an unknown result.
        try:
            for machine in self.machines():
                attempt(lambda m=machine: self.usage(m["id"]))
                attempt(lambda m=machine: self.api("DELETE", "/sandboxes/" + m["id"]))
        except Exception:
            errors.append("cannot confirm machine inventory")
        if self.probe_id:
            attempt(lambda: self.api("DELETE", "/sandboxes/" + self.probe_id))
        def remove_snapshot(name):
            try:
                self.api("DELETE", "/saved-snapshots/" + name)
            except APIError as error:
                if error.status != 404:
                    raise
        attempt(lambda: remove_snapshot(self.probe_snapshot))
        if self.snapshot_owned and self.args.delete_template:
            attempt(lambda: remove_snapshot(self.snapshot))
        if self.environment_id:
            for name in self.secret_names:
                attempt(lambda n=name: self.api(
                    "DELETE", "/environments/" + self.environment_id + "/vars/" + n))
            attempt(lambda: self.api("DELETE", "/environments/" + self.environment_id))
        if self.namespace_owned:
            for suffix in SUFFIXES:
                def remove_namespace(s=suffix):
                    try:
                        self.layer("DELETE", "/v2/namespaces/" + NAMESPACE + s)
                    except APIError as error:
                        if error.status != 404:
                            raise
                attempt(remove_namespace)
                def confirm_namespace(s=suffix):
                    try:
                        self.layer("GET", "/v1/namespaces/" + NAMESPACE + s + "/schema")
                    except APIError as error:
                        if error.status == 404:
                            return
                        raise
                    raise RuntimeError("demo namespace remains")
                attempt(confirm_namespace)
        remaining = self.machines()
        if remaining:
            errors.append("demo machine listing is not empty")
        print("Demo spend: weighted seconds =", sum(v[0] for v in self.usage_by_id.values()),
              "list-price USD =", round(sum(v[1] for v in self.usage_by_id.values()), 6), flush=True)
        print("Final demo-labelled VM listing:", [m["id"] for m in remaining], flush=True)
        if self.snapshot_owned and not self.args.delete_template:
            print("Retained secret-free template:", self.snapshot, flush=True)
        if errors:
            raise RuntimeError("; ".join(errors))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--template", help="reuse a prepared secret-free template")
    parser.add_argument("--delete-template", action="store_true",
                        help="delete a template created by this invocation")
    args = parser.parse_args()
    for name in ("BOAT_API_KEY", "BOAT_SSH_PUBLIC_KEY"):
        if not os.environ.get(name):
            parser.error(name + " must be set")
    if not (os.environ.get("HEVLAYER_API_KEY") or os.environ.get("LAYER_API_KEY")):
        parser.error("HEVLAYER_API_KEY must be set")
    if not (os.environ.get("HEVLAYER_ENDPOINT") or os.environ.get("LAYER_ENDPOINT")):
        parser.error("HEVLAYER_ENDPOINT must be set")
    with tempfile.TemporaryDirectory(prefix="hev-boat-demo-") as root:
        demo = Demo(args, root)
        primary = None
        try:
            demo.run()
        except Exception as error:
            primary = error
        try:
            demo.cleanup()
        except Exception:
            if primary is None:
                raise
            print("Cleanup incomplete; inspect demo-labelled inventory.", flush=True)
        if primary is not None:
            raise primary


if __name__ == "__main__":
    try:
        main()
    except APIError as error:
        raise SystemExit(str(error))
    except (RuntimeError, subprocess.SubprocessError) as error:
        # Do not render subprocess arguments, which could grow to contain secrets.
        raise SystemExit(str(error) if isinstance(error, RuntimeError)
                         else "demo subprocess failed; cleanup attempted")

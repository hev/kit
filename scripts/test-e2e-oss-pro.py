#!/usr/bin/env python3
"""Isolated E2E acceptance for the OSS -> Pro funnel (RFC 0022, memo #555).

    python3 scripts/test-e2e-oss-pro.py PATH/TO/hev

Four temporary homes, never the real one:

  HOME_A       fresh OSS install: `hev up --non-interactive`, the ~/.hev/<surface>
               layout, local read side, a board post and a peer message, `hev s`;
               then the switch to Pro against the stub.
  HOME_LEGACY  pre-populated ~/.hev/archive, ~/dispatch, ~/.local/state/peers-v2:
               found and read in place, nothing abandoned or moved.
  HOME_B       a coworker on the same Pro team: reads what HOME_A wrote.
  HOME_BOH     a shared back-of-house dispatch host: admitted repo runs,
               unadmitted repo fails closed; reads the shared Pro board.
  HOME_TTY     `hev onboard --no-picker` driven through a real PTY, then a
               re-run that must not clobber the operator's edits.

Isolation: every child gets a minimal environment (no inherited HEV_*,
HEVLAYER_*, TMUX*), a private TMUX_TMPDIR and a tmux wrapper on a private
socket, and stub `systemctl`, `launchctl` and `gh` that fail, so no service
manager, real tmux server or GitHub is touched. The Pro side is an in-process
loopback stub (scripts/e2e_pro_stub.py); nothing reaches an external network.
Every process this script starts is stopped by PID.
"""
import json
import os
from pathlib import Path
import pty
import select
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import tomllib
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parent))
from e2e_pro_stub import Stub  # noqa: E402

HEV = str(Path(sys.argv[1]).resolve()) if len(sys.argv) > 1 else sys.exit("usage: test-e2e-oss-pro.py HEV")
TIMEOUT = 90
LEGACY = (".hev/archive", "dispatch", ".local/state/peers-v2")
SURFACES = ("traces", "board", "orders", "peers", "hooks", "evidence")

if not sys.platform.startswith("linux"):
    # On macOS `hev up` drives launchd itself; the stubs here cover Linux.
    print("SKIP: test-e2e-oss-pro.py runs on Linux (CI ubuntu-latest)")
    sys.exit(0)

REAL_TMUX = shutil.which("tmux")
assert REAL_TMUX, "tmux required"
children = []  # Popen objects this run started


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def wait_for(check, label, timeout=30):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        v = check()
        if v:
            return v
        time.sleep(0.1)
    raise AssertionError("timed out: " + label)


def tree(root):
    root = Path(root)
    return {str(p.relative_to(root)): p.stat().st_size for p in root.rglob("*") if p.is_file()} if root.exists() else {}


SESSION = "0c0c0c0c-0000-4000-8000-0000000000%02d"


def seed_transcript(home, n, words):
    """A one-turn Claude Code transcript capture can index."""
    sid = SESSION % n
    p = Path(home, ".claude/projects/-w", sid + ".jsonl")
    p.parent.mkdir(parents=True, exist_ok=True)
    ts = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() - 60))
    rows = [
        {"type": "user", "uuid": "u1", "sessionId": sid, "timestamp": ts, "cwd": "/w",
         "message": {"role": "user", "content": words + " deploy check"}},
        {"type": "assistant", "uuid": "a1", "parentUuid": "u1", "sessionId": sid, "timestamp": ts, "cwd": "/w",
         "requestId": "r1", "message": {"role": "assistant", "model": "claude-sonnet-4-6",
                                        "content": [{"type": "text", "text": words + " looks fine"}],
                                        "usage": {"input_tokens": 7, "output_tokens": 3}}},
    ]
    p.write_text("\n".join(json.dumps(r) for r in rows) + "\n")
    return sid


class Home:
    def __init__(self, root, name, stubbin):
        self.name = name
        self.path = Path(root, name)
        self.path.mkdir(mode=0o700)
        self.env = {
            "HOME": str(self.path), "USERPROFILE": str(self.path),
            "PATH": str(stubbin) + os.pathsep + os.environ.get("PATH", "/usr/bin:/bin"),
            "TMUX_TMPDIR": str(Path(root, "tmux-tmp")), "LANG": "C.UTF-8",
            "GIT_CONFIG_NOSYSTEM": "1", "PEERS_SELF": name.lower(), "PEERS_HOSTS": "",
        }

    def run(self, *args, check=True, env=None, cwd=None, timeout=TIMEOUT):
        e = dict(self.env, **(env or {}))
        r = subprocess.run([HEV, *args], env=e, cwd=cwd or self.path, stdin=subprocess.DEVNULL,
                           capture_output=True, text=True, timeout=timeout)
        if check and r.returncode != 0:
            raise AssertionError(f"[{self.name}] hev {' '.join(args)} -> {r.returncode}\n{r.stdout}\n{r.stderr}")
        return r

    def start(self, *args, env=None, log="proc.log"):
        e = dict(self.env, **(env or {}))
        out = open(self.path.parent / f"{self.name}-{log}", "w")
        p = subprocess.Popen([HEV, *args], env=e, cwd=self.path, stdin=subprocess.DEVNULL,
                             stdout=out, stderr=subprocess.STDOUT)
        children.append(p)
        return p

    def git(self, *args, cwd):
        return subprocess.run(["git", "-c", "user.name=E2E", "-c", "user.email=e2e@example.test", *args],
                              cwd=cwd, env=self.env, check=True, capture_output=True, text=True).stdout


def stop(p):
    if p.poll() is None:
        p.send_signal(signal.SIGTERM)
        try:
            p.wait(10)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait(5)


def http_get(url):
    with urllib.request.urlopen(url, timeout=10) as r:
        return r.status, r.read().decode("utf-8", "replace")


def capture_once(home, env, done, label):
    """Run the capture daemon until done() holds, then stop it."""
    d = home.start("d", env=env, log="hevd.log")
    try:
        wait_for(done, label, timeout=60)
    finally:
        stop(d)


def board_server(home, env):
    port = free_port()
    url = f"http://127.0.0.1:{port}"
    p = home.start("board", "serve", "--addr", f"127.0.0.1:{port}", env=env, log="board.log")

    def up():
        try:
            return http_get(url + "/api/threads?limit=5")[0] == 200
        except OSError:
            return p.poll() is not None and AssertionError((home.path.parent / f"{home.name}-board.log").read_text())
    r = wait_for(up, home.name + " board server")
    assert r is True, r
    return p, url


def main():
    with tempfile.TemporaryDirectory(prefix="hev-e2e-") as tmp:
        root = Path(tmp)
        (root / "tmux-tmp").mkdir(mode=0o700)
        stubbin = root / "bin"
        stubbin.mkdir()
        sock = root / "tmux.sock"

        def script(name, body):
            p = stubbin / name
            p.write_text("#!/bin/sh\n" + body)
            p.chmod(0o755)
        script("tmux", f'exec "{REAL_TMUX}" -S "{sock}" "$@"\n')
        for name in ("systemctl", "launchctl", "gh"):
            script(name, "exit 1\n")
        # The dispatch harness: records that it ran, then exits.
        script("codex", 'echo "$(basename "$(dirname "$PWD")")" >> "$HOME/harness-starts"\n')

        A = Home(root, "HOME_A", stubbin)
        L = Home(root, "HOME_LEGACY", stubbin)
        B = Home(root, "HOME_B", stubbin)
        BOH = Home(root, "HOME_BOH", stubbin)
        TTY = Home(root, "HOME_TTY", stubbin)
        try:
            onboard_pty(TTY)
            with Stub({"key-a": "alice", "key-b": "bob", "key-boh": "boh"}) as stub:
                fresh_oss(A)
                legacy(L)
                pro_switch(A, stub)
                coworker_and_boh(B, BOH, stub, root)
            print("PASS: PTY onboarding + idempotent re-run; fresh OSS layout + local read side + board/peers + hev s; legacy dirs kept and read; "
                  "OSS -> Pro switch off disk; coworker and shared BOH: admission both sides, shared Pro reads")
        finally:
            for p in children:
                stop(p)
            subprocess.run([REAL_TMUX, "-S", str(sock), "kill-server"], capture_output=True)


def fresh_oss(A):
    # 1. hev up with an empty HOME and no TTY: exit 0, no prompt, no TOML edits.
    r = A.run("up", "--non-interactive")
    assert "hev onboard" not in r.stdout and "?" not in r.stdout.replace("http", ""), r.stdout
    for s in SURFACES:
        assert (A.path / ".hev" / s).is_dir(), f"~/.hev/{s} missing"
    for rel in LEGACY:
        assert not (A.path / rel).exists(), f"fresh install created legacy {rel}"
    cfg_path = A.path / ".config/hev/dispatch.toml"
    assert oct(cfg_path.stat().st_mode & 0o777) == "0o600", oct(cfg_path.stat().st_mode)
    cfg = tomllib.loads(cfg_path.read_text())
    assert cfg["default_host"] == "here" and cfg["hosts"]["here"]["local"] is True, cfg
    # A second run is idempotent and keeps the dispatch config byte for byte.
    before = cfg_path.read_bytes()
    A.run("up", "--non-interactive")
    assert cfg_path.read_bytes() == before

    # Local read side: Traces, and no Evidence tab (OSS stays Traces + Board).
    port = free_port()
    serve = A.start("serve", "--port", str(port), log="serve.log")
    status, html = wait_for(lambda: _try_get(f"http://127.0.0.1:{port}/"), "hev serve")
    assert status == 200 and 'data-v="traces"' in html, html[:400]
    assert 'data-v="evidence"' not in html and ">Evidence<" not in html
    A.serve = serve

    # Board: a post on files under ~/.hev/board, read back by CLI and by the
    # board's web API.
    r = A.run("board", "post", "--board", "feedback", "--title", "e2e fresh post", "-m", "written on HOME_A files")
    tid = r.stdout.split("#", 1)[1].split()[0]
    assert "written on HOME_A files" in A.run("board", "read", tid).stdout
    assert any(p.name for p in (A.path / ".hev/board/threads").rglob("*") if p.is_file())
    bserve, burl = board_server(A, {})
    assert "e2e fresh post" in http_get(burl + "/api/threads?limit=5")[1]
    A.board = bserve

    # Peers: a message between two panes on the private tmux socket lands
    # under ~/.hev/peers, never ~/.local/state/peers-v2.
    peers_roundtrip(A, "a-one", "a-two", "hello from HOME_A")
    assert any((A.path / ".hev/peers").rglob("*.jsonl")), tree(A.path / ".hev/peers")
    assert not (A.path / ".local/state/peers-v2").exists()

    # hev s names the files archive under ~/.hev/traces.
    r = A.run("s")
    assert str(A.path / ".hev/traces") in r.stdout, r.stdout


def _try_get(url):
    try:
        return http_get(url)
    except OSError:
        return None


def peers_roundtrip(H, one, two, text):
    panes = {}
    for name in (one, two):
        subprocess.run(["tmux", "-f", "/dev/null", "new-session", "-d", "-s", name, "exec -a claude sleep 300"],
                       env=H.env, check=True)
        panes[name] = subprocess.run(["tmux", "display-message", "-p", "-t", name, "#{pane_id}"], env=H.env,
                                     check=True, capture_output=True, text=True).stdout.strip()
        H.run("peers", "inbox", "on", name)
    H.run("peers", "send", two, text, "--from", one, "--id", "e2e-" + two)
    got = json.loads(H.run("peers", "inbox", "--pending", "--json", env={"TMUX_PANE": panes[two]}).stdout)
    assert [m["id"] for m in got["messages"]] == ["e2e-" + two], got
    H.run("peers", "inbox", "ack", "e2e-" + two, env={"TMUX_PANE": panes[two]})
    for name in (one, two):
        subprocess.run(["tmux", "kill-session", "-t", name], env=H.env, capture_output=True)


def legacy(L):
    # Pre-populate the three legacy directories with data and sentinels.
    for rel in LEGACY:
        (L.path / rel).mkdir(parents=True)
        (L.path / rel / "SENTINEL").write_text("legacy " + rel + "\n")
    run_dir = L.path / "dispatch/old-run"
    run_dir.mkdir()
    (run_dir / "order.md").write_text("---\nname: old-run\nhost: here\n---\nOld work\n")
    (run_dir / "meta.json").write_text(json.dumps({"name": "old-run", "host": "here", "repo": "kit",
                                                   "branch": "d/old-run", "start": "2026-11-01T00:00:00Z"}))
    sentinels = {rel: (L.path / rel / "SENTINEL").read_bytes() for rel in LEGACY}

    L.run("up", "--non-interactive")
    # Legacy surfaces are kept: no unified directory takes over beside them.
    for s in ("traces", "orders", "peers"):
        assert not (L.path / ".hev" / s).exists(), f"~/.hev/{s} created beside its legacy directory"
    for s in ("board", "hooks", "evidence"):
        assert (L.path / ".hev" / s).is_dir()

    # Traces: capture indexes into the legacy archive and search reads it.
    seed_transcript(L.path, 1, "walrus mango")
    sessions = L.path / ".hev/archive/hev-traces/sessions"
    capture_once(L, {}, lambda: sessions.exists() and any(sessions.rglob("*.json")), "legacy capture")
    assert "walrus" in L.run("traces", "search", "walrus mango").stdout.lower()

    # Orders: run records are read from ~/dispatch.
    assert "old-run" in L.run("dispatch", "ls", "--all").stdout
    assert "d/old-run" in L.run("dispatch", "show", "old-run").stdout
    # Peers: messages go to the legacy inbox tree.
    peers_roundtrip(L, "l-one", "l-two", "hello legacy")
    assert any((L.path / ".local/state/peers-v2").rglob("*.jsonl"))
    assert not (L.path / ".hev/peers").exists()
    # Nothing was lost or moved.
    for rel, raw in sentinels.items():
        assert (L.path / rel / "SENTINEL").read_bytes() == raw, rel
    assert (run_dir / "order.md").exists()


def pro_env(stub, key, board_ns="team-board"):
    return {"HEVLAYER_API_KEY": key, "HEVLAYER_ENDPOINT": stub.url,
            "FACTORY_BOARD_LAYER_ENDPOINT": stub.url, "FACTORY_BOARD_NAMESPACE": board_ns}


def pro_switch(A, stub):
    env = pro_env(stub, "key-a")
    local_traces, local_board = tree(A.path / ".hev/traces"), tree(A.path / ".hev/board")

    r = A.run("up", "--non-interactive", env=env)
    assert "hev layer configured" in r.stdout, r.stdout
    cfg = tomllib.loads((A.path / ".hev/config.toml").read_text())
    assert cfg["layer"]["store"] == "hosted" and cfg["layer"]["endpoint"] == stub.url, cfg["layer"]
    # storagemode flips: this shell now resolves hev layer, not files.
    s = A.run("s", env=env).stdout
    assert "hev layer namespace hev-traces at " + stub.url in s, s
    # The local read side stops: in Pro the dashboard is hosted (`hev open`).
    # (Under systemd/launchd `hev up` unloads it; here we own the process.)
    assert "hev open" in r.stdout
    stop(A.serve)
    stop(A.board)

    # Trace writes and reads go to the stub, under HOME_A's key.
    seed_transcript(A.path, 2, "zebra pineapple")
    config_path = A.path / ".hev/config.toml"
    key_path = config_path.with_name("layer.key")
    key_path.touch(mode=0o600)
    key_path.write_text(env["HEVLAYER_API_KEY"] + "\n")
    text = config_path.read_text()
    inline_key = next(line for line in text.splitlines() if line.startswith("api_key = "))
    config_path.write_text(text.replace(inline_key, 'api_key_file = "layer.key"', 1))
    capture_env = {name: value for name, value in env.items() if name != "HEVLAYER_API_KEY"}
    capture_once(A, capture_env, lambda: stub.writes("alice"), "Pro disk bootstrap capture")
    stub_rows = stub.namespaces()
    assert stub_rows.get("hev-traces", 0) > 0, stub_rows
    n = len(stub.requests)
    A.run("traces", "search", "zebra pineapple", env=env)
    assert any(q["path"].startswith("/v2/namespaces/hev-traces") and q["account"] == "alice"
               for q in stub.requests[n:]), stub.requests[n:]

    # Board writes go to the stub through a keyed board server.
    bserve, burl = board_server(A, env)
    A.run("board", "post", "--board", "feedback", "--title", "e2e pro post", "-m", "written to Pro",
          env=dict(env, FACTORY_BOARD_URL=burl))
    assert stub.namespaces().get("team-board", 0) > 0, stub.namespaces()
    stop(bserve)

    # The Pro off-disk invariant: no new local archive or board files.
    assert tree(A.path / ".hev/traces") == local_traces, "Pro mode wrote local trace files"
    assert tree(A.path / ".hev/board") == local_board, "Pro mode wrote local board files"
    assert not (A.path / ".hev/archive").exists()


def coworker_and_boh(B, BOH, stub, root):
    # HOME_B (coworker) reads the shared Pro board HOME_A wrote.
    benv = pro_env(stub, "key-b")
    B.run("up", "--non-interactive", env=benv)
    bserve, burl = board_server(B, benv)
    assert "e2e pro post" in B.run("board", "ls", env=dict(benv, FACTORY_BOARD_URL=burl)).stdout
    stop(bserve)
    assert not any((B.path / ".hev/board").rglob("threads/*")), "coworker read wrote local board files"

    # HOME_BOH: a shared dispatch host whose own config admits one repo.
    henv = pro_env(stub, "key-boh")
    BOH.run("up", "--non-interactive", env=henv)
    src = {}
    for name in ("admitted", "unadmitted"):
        s = root / f"src-{name}"
        s.mkdir()
        BOH.git("init", "-q", "-b", "main", cwd=s)
        BOH.git("commit", "-q", "--allow-empty", "-m", "initial", cwd=s)
        clone = BOH.path / "workspace" / name
        clone.parent.mkdir(exist_ok=True)
        subprocess.run(["git", "clone", "-q", str(s), str(clone)], env=BOH.env, check=True)
        src[name] = clone
    cfg = BOH.path / ".config/hev/dispatch.toml"
    cfg.write_text(cfg.read_text() + f'\n[admission]\nrepos = ["{root / "src-admitted"}"]\n')

    def order(name, repo):
        p = root / f"{name}.md"
        p.write_text(f"---\nname: {name}\nrepo: {repo}\nhost: here\nharness: codex\n---\nTask\n## Acceptance\n- done\n")
        return str(p)
    ok = BOH.run("dispatch", order("boh-admitted", src["admitted"]))
    assert "started boh-admitted" in ok.stdout, ok.stdout + ok.stderr
    starts = BOH.path / "harness-starts"
    wait_for(lambda: starts.exists() and "boh-admitted" in starts.read_text(), "admitted run starts")
    assert (BOH.path / ".hev/orders/boh-admitted/meta.json").exists()
    refused = BOH.run("dispatch", order("boh-unadmitted", src["unadmitted"]), check=False)
    assert refused.returncode != 0 and "not admitted" in (refused.stdout + refused.stderr), refused
    assert "boh-unadmitted" not in starts.read_text()
    wait_for(lambda: _result_done(BOH.path / ".hev/orders/boh-admitted/result.json"), "admitted run result")

    # BOH reads the same shared Pro board.
    bserve, burl = board_server(BOH, henv)
    assert "e2e pro post" in BOH.run("board", "ls", env=dict(henv, FACTORY_BOARD_URL=burl)).stdout
    stop(bserve)
    accounts = {q["account"] for q in stub.requests if q["namespace"] == "team-board"}
    assert {"alice", "bob", "boh"} <= accounts, accounts
    # Wait for the run's supervisor to leave the private tmux server.
    wait_for(lambda: subprocess.run(["tmux", "has-session", "-t", "d-boh-admitted"], env=BOH.env,
                                    capture_output=True).returncode != 0, "supervisor exits")


def pty_session(H, args, answers, timeout=60):
    """Run hev on a real PTY, answering each prompt (a substring) in turn."""
    master, slave = pty.openpty()
    p = subprocess.Popen([HEV, *args], env=dict(H.env, TERM="dumb"), cwd=H.path,
                         stdin=slave, stdout=slave, stderr=slave, close_fds=True)
    children.append(p)
    os.close(slave)
    out, pending = b"", list(answers)
    deadline = time.monotonic() + timeout
    try:
        while time.monotonic() < deadline:
            r, _, _ = select.select([master], [], [], 0.2)
            if r:
                try:
                    chunk = os.read(master, 4096)
                except OSError:
                    break
                if not chunk:
                    break
                out += chunk
            while pending and pending[0][0].encode() in out.split(b"\x00")[-1]:
                prompt, answer = pending.pop(0)
                out += b"\x00"  # mark: later prompts must appear after this answer
                os.write(master, answer.encode() + b"\r")
            if p.poll() is not None and not r:
                break
        assert p.wait(10) == 0, out.decode(errors="replace")
        assert not pending, f"never asked {pending}:\n{out.decode(errors='replace')}"
        return out.decode(errors="replace").replace("\x00", "")
    finally:
        os.close(master)
        stop(p)


def onboard_pty(T):
    # Without a terminal, onboard refuses rather than waiting on stdin.
    r = T.run("onboard", "--no-picker", check=False)
    assert r.returncode != 0 and "needs a terminal" in r.stderr + r.stdout, r
    (T.path / ".codex").mkdir()  # a harness kit can detect
    out = pty_session(T, ["onboard", "--no-picker", "--no-dashboard"], [
        ("Use which harnesses?", "nope"),          # refused, asked again
        ("Use which harnesses?", "codex"),
        ("Workspace directory", "~/code"),
        ("Create the workspace", "y"),
    ])
    assert 'unknown harness "nope"' in out and "[✓] codex" in out and "Next steps:" in out, out
    state = tomllib.loads((T.path / ".hev/onboard.toml").read_text())
    assert state["harnesses"] == ["codex"] and state["workspace"] == "~/code", state
    assert oct((T.path / ".hev/onboard.toml").stat().st_mode & 0o777) == "0o600"
    cfg = T.path / ".config/hev/dispatch.toml"
    assert tomllib.loads(cfg.read_text())["hosts"]["here"]["workspace"] == "~/code"
    assert (T.path / "code").is_dir() and all((T.path / ".hev" / s).is_dir() for s in SURFACES)
    # Re-run: Enter keeps every answer; the operator's dispatch edit survives.
    cfg.write_text(cfg.read_text() + "# operator note\n")
    edited = cfg.read_bytes()
    out = pty_session(T, ["onboard", "--no-picker", "--no-dashboard"], [
        ("Use which harnesses?", ""), ("Workspace directory", ""), ("Create the workspace", ""),
    ])
    assert "onboarded before" in out and "[codex]" in out and "[~/code]" in out, out
    assert cfg.read_bytes() == edited
    # A configured machine: `hev up` on a terminal no longer onboards.
    out = pty_session(T, ["up", "--no-dashboard"], [])
    assert "hev onboard" not in out, out


def _result_done(p):
    try:
        return json.loads(p.read_text()).get("end", "0001").startswith("20")
    except (OSError, ValueError):
        return False


if __name__ == "__main__":
    main()

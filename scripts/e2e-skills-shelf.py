#!/usr/bin/env python3
"""Live end-to-end check of the skills shelf (RFC 0018) in a temporary HOME.

Usage: scripts/e2e-skills-shelf.py HEV_BINARY [MODEL]

Needs network (GitHub), git, tmux and an authenticated `claude` CLI. It never
touches the operator's skills roots, dispatch config or tmux server: HOME,
HEV_CONFIG and HEV_DISPATCH_CONFIG point into a temporary directory and tmux
runs on a private socket. When HEVLAYER_API_KEY is set (for example through
`hev secrets run --ref HEVLAYER_API_KEY=...`), the skills collection is
written to and searched in that Layer namespace (HEVLAYER_NAMESPACE, default
hev-traces, so <namespace>-skills); otherwise the local lexical index is used.

Steps: add one mode pack and one tool pack, list the shelf, dispatch a scratch
run enabling one skill from each and a second run enabling none, then report
which skills each worker's harness listed at start (Claude's init event), which
it loaded (the transcript's skill_loads, read by hev skills-preview), whether
the links were cleaned up, and a skills search hit.
"""
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import time

# The mode skill the worker invokes must allow model invocation: pstack's
# poteto-mode sets disable-model-invocation, so whether a transcript records
# its load is up to the model. pstack is still added, as the second mode the
# one-mode rule refuses.
MODE_PACK = ("obra/superpowers//skills", "mode", "superpowers", "brainstorming")
SECOND_MODE = ("cursor/plugins//pstack", "mode", "pstack", "poteto-mode")
TOOL_PACK = ("mattpocock/skills//skills", "tool", "mattpocock", "tdd")


def main():
    hev = str(Path(sys.argv[1]).resolve())
    model = sys.argv[2] if len(sys.argv) > 2 else "claude-haiku-5-5"
    real_tmux = shutil.which("tmux")
    claude = shutil.which("claude")
    assert real_tmux and claude, "tmux and claude are required"
    keep = os.environ.get("KEEP_E2E") == "1"
    root = Path(tempfile.mkdtemp(prefix="shelf-e2e-", dir="/tmp"))
    home, binary, workspace = root / "home", root / "bin", root / "workspace"
    for p in (home, binary, workspace):
        p.mkdir()
    socket = str(root / "tmux.sock")
    env = {**os.environ, "HOME": str(home), "USERPROFILE": str(home),
           "PATH": str(binary) + os.pathsep + os.environ["PATH"],
           "HEV_CONFIG": str(root / "config.toml"), "GIT_CONFIG_NOSYSTEM": "1",
           "HEV_PEERS_BACKEND": "", "HEV_PEERS_STATE": str(root / "peers"), "PEERS_SELF": "shelf-e2e"}
    for k in ("TMUX", "TMUX_PANE", "HEVLAYER_API_KEY_REF", "HEV_ORCHESTRATOR", "CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT"):
        env.pop(k, None)
    layer = bool(env.get("HEVLAYER_API_KEY"))
    namespace = env.get("HEVLAYER_NAMESPACE", "hev-traces")
    config = root / "config.toml"
    if layer:
        endpoint = env.get("HEVLAYER_ENDPOINT", "https://aws-us-east-1.hevlayer.com")
        config.write_text(f"[layer]\nendpoint = {json.dumps(endpoint)}\nnamespace = {json.dumps(namespace)}\n\n")
    dconfig = home / ".config/hev/dispatch.toml"
    dconfig.parent.mkdir(parents=True)
    dconfig.write_text(f"cap = 2\nworkspace = '{workspace}'\ndefault_host = 'local'\n[hosts.local]\nlocal = true\n")
    env["HEV_DISPATCH_CONFIG"] = str(dconfig)
    (binary / "tmux").write_text(f"#!/bin/sh\nexec {shlex.quote(real_tmux)} -S {shlex.quote(socket)} \"$@\"\n")
    (binary / "tmux").chmod(0o700)
    (binary / "hev").symlink_to(hev)
    evidence = {"root": str(root), "store": "layer:" + namespace + "-skills" if layer else "local lexical index"}

    def run(*args, check=True):
        r = subprocess.run(args, env=env, text=True, capture_output=True)
        if check and r.returncode != 0:
            raise AssertionError(f"{args}: exit {r.returncode}\n{r.stdout}\n{r.stderr}")
        return r

    try:
        # 1. Two real packs: one mode, one tool.
        for spec, kind, name, _ in (MODE_PACK, TOOL_PACK):
            out = run(hev, "skills", "add", spec, "--kind", kind, "--name", name)
            print(f"$ hev skills add {spec} --kind {kind} --name {name}  (exit 0)\n{out.stdout}{out.stderr}")
        shelf = run(hev, "skills", "shelf")
        print("$ hev skills shelf  (exit 0)\n" + shelf.stdout)
        packs = {p["pack"]: p for p in json.loads(run(hev, "skills", "shelf", "--json").stdout)}
        evidence["shelf"] = {n: {"commit": p["commit"], "license": p["license"], "kind": p["kind"], "author": p["author"], "skills": len(p["skills"])} for n, p in packs.items()}
        # Installed is not active: nothing in the temporary harness roots.
        for d in (".claude/skills", ".codex/skills", ".agents/skills"):
            assert not any((home / d).glob("*")) if (home / d).exists() else True, d

        # 2. A scratch repository with an origin, for dispatch.
        repo = workspace / "scratch"
        run("git", "init", "-b", "main", str(repo))
        (repo / "README.md").write_text("scratch repository for the skills shelf e2e\n")
        run("git", "-C", str(repo), "add", ".")
        run("git", "-C", str(repo), "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
        origin = root / "origin.git"
        run("git", "init", "--bare", "-b", "main", str(origin))
        run("git", "-C", str(repo), "remote", "add", "origin", str(origin))
        run("git", "-C", str(repo), "push", "-u", "origin", "main")

        mode_ref = f"{MODE_PACK[2]}/{MODE_PACK[3]}"
        tool_ref = f"{TOOL_PACK[2]}/{TOOL_PACK[3]}"
        body = f"""
Scratch run for the skills shelf end-to-end check. Do not commit, push or open
a pull request; these instructions override the defaults.

1. List every skill available to you whose name is {MODE_PACK[3]} or {TOOL_PACK[3]}.
2. Invoke each of those you have with the Skill tool, one after the other. Do not
   follow their instructions beyond reading them.
3. Write ../report.md naming the skills from step 1 you had and invoked, then
   make the last line of ../status.md exactly `done`.

## Acceptance
- ../report.md lists which of the two skills were available and invoked.
"""
        runs = {"shelf-e2e-with": f"skills: [{mode_ref}, {tool_ref}]\n", "shelf-e2e-without": ""}
        for name, skills in runs.items():
            order = root / f"{name}.md"
            order.write_text(f"---\nname: {name}\nhost: local\nrepo: scratch\nbase: main\nharness: claude\nmodel: {model}\nvalidation: none\ntimeout: 20\n{skills}---\n{body}")
            out = run(hev, "dispatch", str(order))
            print(f"$ hev dispatch {order.name}  (exit 0)\n{out.stdout}{out.stderr}")
            if name == "shelf-e2e-with":
                meta = json.loads((home / ".hev/orders" / name / "meta.json").read_text())
                evidence["meta_shelf_skills"] = meta.get("shelf_skills")

        # A third order enabling two modes is refused before it runs.
        bad = root / "two-modes.md"
        run(hev, "skills", "add", SECOND_MODE[0], "--kind", SECOND_MODE[1], "--name", SECOND_MODE[2])
        bad.write_text(f"---\nname: shelf-e2e-two-modes\nhost: local\nrepo: scratch\nbase: main\nharness: claude\nmodel: {model}\nvalidation: none\nskills: [{mode_ref}, {SECOND_MODE[2]}/{SECOND_MODE[3]}]\n---\nnothing\n\n## Acceptance\n- refused\n")
        refused = run(hev, "dispatch", str(bad), check=False)
        evidence["two_modes"] = {"exit": refused.returncode, "error": (refused.stderr or refused.stdout).strip().splitlines()[-1:]}
        print(f"$ hev dispatch two-modes.md  (exit {refused.returncode})\n{refused.stdout}{refused.stderr}")

        deadline = time.monotonic() + 25 * 60
        for name in runs:
            result = home / ".hev/orders" / name / "result.json"
            while True:
                try:
                    r = json.loads(result.read_text())
                    if not r.get("end", "0001").startswith("0001"):
                        break
                except (FileNotFoundError, json.JSONDecodeError):
                    pass
                assert time.monotonic() < deadline, f"{name} did not finish"
                time.sleep(5)
            evidence.setdefault("runs", {})[name] = {"exit_code": r.get("exit_code"), "session": r.get("session_id")}

        # 3. What each worker's harness saw (init event) and loaded (transcript).
        for name in runs:
            init = None
            for line in (home / ".hev/orders" / name / "log").read_text(errors="replace").splitlines():
                try:
                    d = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if d.get("type") == "system" and d.get("subtype") == "init":
                    init = d
                    break
            assert init, f"{name}: no init event"
            evidence["runs"][name]["init_skills"] = sorted(init.get("skills") or [])
            wt = home / ".hev/orders" / name / "wt"
            evidence["runs"][name]["left_in_worktree"] = sorted(str(p.relative_to(wt)) for p in wt.glob(".claude/**/*"))
            evidence["runs"][name]["skills_record_left"] = (home / ".hev/orders" / name / "skills.json").exists()
            evidence["runs"][name]["git_status"] = run("git", "-C", str(wt), "status", "--porcelain").stdout.strip()
        with_, without = evidence["runs"]["shelf-e2e-with"], evidence["runs"]["shelf-e2e-without"]
        evidence["init_skills_only_in_with_run"] = sorted(set(with_["init_skills"]) - set(without["init_skills"]))
        evidence["init_skills_only_in_without_run"] = sorted(set(without["init_skills"]) - set(with_["init_skills"]))
        (root / "none").mkdir(exist_ok=True)
        plan = root / "skills-plan.json"
        preview = run(hev, "skills-preview", "--claude-root", str(home / ".claude/projects"), "--codex-root", str(root / "none"),
                      "--pi-root", str(root / "none"), "--opencode-root", str(root / "none"), "--dot-root", str(root / "none"), "--out", str(plan))
        evidence["skills_preview"] = json.loads(preview.stdout)
        p = json.loads(plan.read_text())
        loads = {}
        for s in p.get("sessions", []):
            v = s.get("skills") or []
            loads[s.get("session_id")] = json.loads(v) if isinstance(v, str) else v
        for name in runs:
            evidence["runs"][name]["transcript_skill_loads"] = loads.get(evidence["runs"][name]["session"], [])

        # 4. The skills collection.
        hits = json.loads(run(hev, "skills", "search", "--json", "--top", "3", "write the failing test first red green refactor").stdout)
        evidence["search"] = [{k: h[k] for k in ("pack", "skill", "commit", "license", "author", "path")} for h in hits]

        ok = (evidence["init_skills_only_in_with_run"] == sorted([MODE_PACK[3], TOOL_PACK[3]])
              and not evidence["init_skills_only_in_without_run"]
              and sorted(with_["transcript_skill_loads"] or []) == sorted([MODE_PACK[3], TOOL_PACK[3]])
              and not (without["transcript_skill_loads"] or [])
              and not with_["left_in_worktree"] and not with_["skills_record_left"] and not with_["git_status"]
              and evidence["two_modes"]["exit"] != 0
              and any(h["commit"] and h["license"] and h["pack"] for h in evidence["search"]))
        evidence["pass"] = ok
        print(json.dumps(evidence, indent=2))
        return 0 if ok else 1
    finally:
        subprocess.run([real_tmux, "-S", socket, "kill-server"], capture_output=True)
        if not keep:
            shutil.rmtree(root, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())

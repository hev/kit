#!/usr/bin/env python3
"""Live skill-source acceptance; every install uses an isolated temporary HOME."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

binary = str(Path(sys.argv[1]).resolve())
with tempfile.TemporaryDirectory(prefix="skill-source-acceptance-") as tmp:
    root = Path(tmp)
    home = root / "home"
    repo = root / "repo"
    home.mkdir()
    repo.mkdir()
    env = {**os.environ, "HOME": str(home), "USERPROFILE": str(home),
           "HEV_CONFIG": str(home / "config.toml"), "GIT_CONFIG_NOSYSTEM": "1",
           "GIT_CONFIG_GLOBAL": os.devnull}

    def git(*args):
        return subprocess.check_output(["git", "-C", str(repo), *args], env=env, text=True).strip()

    def write(path, text):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def commit():
        git("add", ".")
        git("-c", "commit.gpgsign=false", "commit", "-m", "Update example skills")
        return git("rev-parse", "HEAD")

    def hev(action):
        result = subprocess.run([binary, "skills", action], env=env, text=True,
                                capture_output=True)
        if result.returncode:
            raise AssertionError(result.stdout + result.stderr)
        return result.stdout

    git("init", "-b", "main")
    git("config", "user.name", "Example")
    git("config", "user.email", "example@example.invalid")
    skill = repo / "skills" / "example" / "SKILL.md"
    write(skill, "---\nmetadata:\n  harnesses: all\n---\nFirst revision\n")
    command = repo / "skills" / "example" / "bin" / "example-command.cmd"
    write(command, "@echo example\n")
    command.chmod(0o755)
    first = commit()
    # JSON strings are also valid TOML basic strings (including Windows paths).
    write(home / "config.toml", "[[skills.sources]]\npath = " + json.dumps(str(repo)) + "\nsubdir = 'skills'\n")
    hev("sync")
    hev("check")
    installs = [home / f".{h}" / "skills" / "example" for h in ("claude", "codex", "agents")]
    for path in installs:
        assert "First revision" in (path / "SKILL.md").read_text()
        assert os.name == "nt" or path.is_symlink()
    print("PASS: first commit pinned; all three harness installs and bin command exist")
    assert (home / ".local/bin/example-command.cmd").exists()
    old = installs[0].resolve()
    write(skill, "---\nharnesses: all\n---\nSecond revision\n")
    second = commit()
    assert first != second
    hev("sync")
    hev("check")
    for path in installs:
        assert "Second revision" in (path / "SKILL.md").read_text()
    if os.name != "nt":
        assert "First revision" in (old / "SKILL.md").read_text()
    print("PASS: fast-forward update installed; previous Unix pin retains first revision")
    git("rm", "-r", "skills/example")
    write(repo / "skills" / "keep" / "SKILL.md", "A surviving skill\n")
    commit()
    hev("sync")
    hev("check")
    for path in [*installs, home / ".local/bin/example-command.cmd"]:
        assert not os.path.lexists(path), path
    print("PASS: removed skill and command pruned from all harnesses")
    print("PASS: all CLI runs used throwaway HOME and HEV_CONFIG")

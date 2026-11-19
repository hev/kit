"""Measure full process startup and cached rendering in a release build."""
import json
import os
from pathlib import Path
import statistics
import subprocess
import sys
import tempfile
import time


def time_command(command, env, samples):
    times = []
    for _ in range(samples):
        start = time.perf_counter()
        subprocess.run(command, env=env, stdout=subprocess.DEVNULL, check=True)
        times.append((time.perf_counter() - start) * 1000)
    return times


def main():
    binary = str(Path(sys.argv[1]).resolve())
    with tempfile.TemporaryDirectory() as tmp:
        cache = Path(tmp) / "snapshot.json"
        cache.write_text(json.dumps({
            "Panes": [
                {"ID": f"${i}", "Name": f"foh-hev-{i}", "Workspace": "hev",
                 "Harness": "codex", "Title": f"Project {i}", "Prompt": "A" * 200}
                for i in range(100)
            ],
            "Peers": 100,
            "Hosts": [{"Name": "mini", "OS": "darwin", "State": "alive", "Activity": "2w/1i"}],
        }))
        env = dict(os.environ, HOME=tmp, HEV_CONFIG=str(Path(tmp) / "config.toml"),
                   HEV_PICK_CACHE=str(cache))
        for args, expected in ((["--emit-items"], b"Project 99"), (["--status", "right"], b"peers 100")):
            output = subprocess.check_output([binary, *args], env=env)
            if expected not in output:
                raise SystemExit(f"{args} did not render the warm cache")
            warm = time_command([binary, *args], env, 25)
            median = statistics.median(warm)
            print(f"{' '.join(args)} warm median {median:.2f} ms, max {max(warm):.2f} ms")
            if median >= 20:
                raise SystemExit(f"{args} exceeded the 20 ms warm median budget")
        cache.unlink()
        for args in (["--emit-items"], ["--status", "right"]):
            cold = time_command([binary, *args], env, 25)
            print(f"{' '.join(args)} no-cache median {statistics.median(cold):.2f} ms, max {max(cold):.2f} ms")


if __name__ == "__main__":
    main()

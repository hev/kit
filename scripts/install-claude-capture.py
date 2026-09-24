#!/usr/bin/env python3
"""Install safe-by-default Claude Code telemetry settings for hev kit.

Writes the OTel environment into ~/.claude/settings.json. Prompt, tool-content,
and raw-body logging stay off by default; users can opt into full-fidelity
capture in their hev config. Also prunes any stale hev hook entries left over
from older installs — hev kit captures traces through OTLP only now.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path


STALE_HOOK_BASENAMES = {"trace-hook.py", "trace-prompt.py", "trace-response.py"}


def load_settings(path: Path) -> dict:
    if not path.exists():
        return {}
    return json.loads(path.read_text())


def is_stale_hev_hook(command: str) -> bool:
    if not command:
        return False
    for name in STALE_HOOK_BASENAMES:
        if command.endswith("/hooks/" + name) or command.endswith("\\hooks\\" + name):
            return True
    return False


def prune_stale_hooks(settings: dict) -> int:
    hooks = settings.get("hooks")
    if not isinstance(hooks, dict):
        return 0

    removed = 0
    for event in list(hooks.keys()):
        entries = hooks.get(event)
        if not isinstance(entries, list):
            continue
        new_entries = []
        for entry in entries:
            sub = entry.get("hooks", []) if isinstance(entry, dict) else []
            kept = []
            for hook in sub:
                cmd = hook.get("command", "") if isinstance(hook, dict) else ""
                if is_stale_hev_hook(cmd):
                    removed += 1
                    continue
                kept.append(hook)
            if kept:
                entry["hooks"] = kept
                new_entries.append(entry)
        if new_entries:
            hooks[event] = new_entries
        else:
            del hooks[event]

    if not hooks:
        settings.pop("hooks", None)
    return removed


def main() -> int:
    home = Path.home()
    settings_path = home / ".claude" / "settings.json"
    raw_bodies_dir = home / ".hev" / "claude-code" / "raw-api-bodies"
    raw_bodies_dir.mkdir(parents=True, exist_ok=True)
    settings_path.parent.mkdir(parents=True, exist_ok=True)

    settings = load_settings(settings_path)
    pruned = prune_stale_hooks(settings)

    env = settings.setdefault("env", {})
    env.update({
        "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
        "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": "1",
        "HEV_CLAUDE_RAW_BODIES_DIR": str(raw_bodies_dir),
        "HEV_OTEL_LISTEN": "127.0.0.1",
        "HEV_OTEL_PORT": "4318",
        "OTEL_LOGS_EXPORTER": "otlp",
        "OTEL_METRICS_EXPORTER": "otlp",
        "OTEL_TRACES_EXPORTER": "otlp",
        "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
        "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": "http://127.0.0.1:4318/v1/logs",
        "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT": "http://127.0.0.1:4318/v1/metrics",
        "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": "http://127.0.0.1:4318/v1/traces",
        "OTEL_LOG_USER_PROMPTS": "0",
        "OTEL_LOG_TOOL_DETAILS": "0",
        "OTEL_LOG_TOOL_CONTENT": "0",
        "OTEL_LOG_RAW_API_BODIES": "",
    })

    settings_path.write_text(json.dumps(settings, indent=4) + "\n")
    print(f"Updated {settings_path}")
    print(f"Raw Claude API bodies disabled by default: {raw_bodies_dir}")
    if pruned:
        print(f"Pruned {pruned} stale hev hook entr{'y' if pruned == 1 else 'ies'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

# kit-skill-capture: rework guidance (FOH, 2026-10-29)

Two independent Mini validations failed. Fix all four defects below in one candidate.

Start from commit c54a584865cf7db7439cd72bfdbba7854b183c3b (branch candidates/kit-skill-capture-v2 on origin). It already fixed the first two defects; the later fresh rework (e504efc) regressed. Fetch it into your clone if it is missing and build on it rather than starting over.

1. Shell comments must not count: `cat /repo/README.md # /repo/skills/x/SKILL.md` loads nothing.
2. Quoted paths with spaces keep the full path: `cat "/repo with spaces/skills/actual/SKILL.md"` gives path `/repo with spaces/skills/actual/SKILL.md`, source `/repo with spaces/skills`.
3. Claude's native Skill tool call must produce a skill load. The skill name is in the tool input (`{"skill":"browser-use"}`). renderToolUse drops it today (it keeps only command/path/file_path/query/pattern), so capture it from the raw input before rendering, or keep the `skill` key. Kit currently stores just "Skill"; that is the bug Adam saw. Test with a fixture whose only skill evidence is the Skill tool call (no extra Read of SKILL.md).
4. Only operands of read commands count: a README read followed by `echo /repo/skills/review/SKILL.md` loads nothing. Parse the command into commands and operands (respect quotes, pipes, &&, ;, comments) and match SKILL.md paths only as operands of read-like commands (cat, sed, head, tail, less, bat, nl, awk on a file, rg/grep on a file) or file-read tool calls.

Add fixture tests for every case above, positive and negative, per harness. Keep everything else from the order (session skills filterable list, dashboard filter, preview command, no live writes).

## Validator findings, run :2
```json
{
  "verdict": "fail",
  "candidate": "2883d15d5f43994c623b9888e2c3dce81ead0a16",
  "context_digest": "75601a3d69a0f5676b98fa0373c2168d2ec6d3e0c8f07399095cef40302f9f1d",
  "findings": [
    {
      "priority": "P2",
      "location": "internal/trace/skills.go:141-147",
      "problem": "Shell comments are scanned as read operands. cat /repo/README.md # /repo/skills/not-loaded/SKILL.md records not-loaded although the only read is ordinary README.md. This contaminates session skills and backfill counts.",
      "expected": "No skill load for this command; parse shell operands/comments before extracting skill paths."
    },
    {
      "priority": "P2",
      "location": "internal/trace/skills.go:34,113-123",
      "problem": "Quoted skill paths containing spaces are truncated. cat \"/repo with spaces/skills/actual/SKILL.md\" records path spaces/skills/actual/SKILL.md and source spaces/skills.",
      "expected": "Preserve full /repo with spaces/skills/actual/SKILL.md and /repo with spaces/skills source."
    }
  ],
  "checks": {
    "git_diff_check": "exit 0; no output (working tree and base-to-candidate diff)",
    "go_test_all": "ok  \tgithub.com/hev/kit/cmd/hev\t(cached)\nok  \tgithub.com/hev/kit/internal/cost\t(cached)\nok  \tgithub.com/hev/kit/internal/daemon\t(cached)\nok  \tgithub.com/hev/kit/internal/index\t(cached)\nok  \tgithub.com/hev/kit/internal/layer\t(cached)\nok  \tgithub.com/hev/kit/internal/local\t(cached)\nok  \tgithub.com/hev/kit/internal/platformops\t(cached)\nok  \tgithub.com/hev/kit/internal/redact\t(cached)\nok  \tgithub.com/hev/kit/internal/redact/fixture\t(cached)\nok  \tgithub.com/hev/kit/internal/serve\t(cached)\nok  \tgithub.com/hev/kit/internal/store\t(cached)\nok  \tgithub.com/hev/kit/internal/trace\t(cached)\n?   \tgithub.com/hev/kit/internal/version\t[no test files]\nok  \tgithub.com/hev/kit/pkg/dashboard\t(cached)\nok  \tgithub.com/hev/kit/pkg/nativejob\t(cached)\nok  \tgithub.com/hev/kit/pkg/nodeinventory\t(cached)\nok  \tgithub.com/hev/kit/pkg/outboxrelay\t(cached)\nok  \tgithub.com/hev/kit/pkg/search\t(cached)\n?   \tgithub.com/hev/kit/skills\t[no test files]\n",
    "independent_boundary_probe": "=== RUN   TestIndependentSkillBoundaryEvidence\n    validation_independent_test.go:6: command=\"cat /repo/README.md # /repo/skills/not-loaded/SKILL.md\" events=[{Name:not-loaded Path:/repo/skills/not-loaded/SKILL.md Source:/repo/skills Turn:independent Seq:0 Time: Commit:}]\n    validation_independent_test.go:6: command=\"cat \\\"/repo with spaces/skills/actual/SKILL.md\\\"\" events=[{Name:actual Path:spaces/skills/actual/SKILL.md Source:spaces/skills Turn:independent Seq:0 Time: Commit:}]\n--- PASS: TestIndependentSkillBoundaryEvidence (0.00s)\nPASS\nok  \tgithub.com/hev/kit/internal/trace\t0.245s\n",
    "fixture_preview": "exit 0; claude_code, codex, pi and dot each sessions=1, would_gain_skills=1",
    "parser_preview_reproduction": "Codex preview of boundaries.jsonl reports actual, multiline, not-loaded; not-loaded is a false positive."
  },
  "review": "Session skills schema explicitly filterable true; skill_loads schema explicitly false. Dashboard has skill display, picker and server filtering; persistence and server unit tests pass. Documented local-only skills-preview has no store client. Historical commit remains unknown. Existing fixtures are explicitly synthetic, not collected real transcripts.",
  "limits": "No live namespace/schema writes, credential resolution, push, merge, deploy, release, shared service changes or acceptance. Browser acceptance was not attempted after independently reproducing blocking parser defects. Candidate unchanged; findings committed separately for immutable evidence."
}
```

## Validator findings, run :6
```json
{
  "verdict": "fail",
  "candidate": "e504efcdfc9d6a0e9bc8c3f459f9a3b82839cac7",
  "context_digest": "cd1930c867350bae89cd2b86d513026210acf0c94553e54519de209eb0b1a1d1",
  "checks": {
    "go test ./...": "pass; output in go-test.txt",
    "git diff --check BASE CANDIDATE": "pass; empty diff-check.txt"
  },
  "findings": [
    {
      "severity": "blocking",
      "location": "internal/trace/skills.go:59; internal/trace/claude.go:271",
      "problem": "Claude Skill input is lost by renderToolUse, which only retains command/path/file_path/query/pattern. captureSkills then tries to decode input JSON from text \"Skill\", so native Skill invocations produce no event or session skill. Fixture test is masked by an additional Read of the same skill.",
      "reproduce": "go run ./cmd/hev skills-preview claude_code .validation-skills/claude-path.jsonl",
      "observed": "sessions_with_skills=0; skills=[]",
      "expected": "review event and session skill; recorded provenance path should be retained when available"
    },
    {
      "severity": "high",
      "location": "internal/trace/skills.go:67-80",
      "problem": "Detector matches any skill path anywhere in a shell command if any read command appears. A README read followed by echo of a SKILL.md path incorrectly counts a skill load.",
      "reproduce": "go run ./cmd/hev skills-preview codex .validation-skills/codex-false-positive.jsonl",
      "observed": "sessions_with_skills=1; skills=[review]",
      "expected": "sessions_with_skills=0; skills=[]"
    }
  ],
  "scope": "Exact candidate inspected, no implementation changes, no live namespace access or writes, no GUI or browser check; blocked on parser correctness. Existing dashboard rollup/filter code and local preview inspected statically."
}
```

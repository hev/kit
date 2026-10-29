# kit-skill-capture: rework guidance v2 (FOH, 2026-10-29)

Start from 82fece319dfe29c9e58ba24d32f06dac7bdfd174 (origin branch candidates/kit-skill-capture-v4). It passes every earlier finding; keep all of it. Fix only the two Codex JavaScript-wrapper cases below, then add the validator's probe as a permanent test.

1. Codex wraps exec calls in JavaScript, including `text(await tools.exec_command({cmd:"cat /repo/skills/actual/SKILL.md"}));`. Recognise any executable `tools.exec_command({cmd: ...})` call regardless of the surrounding wrapper (text(...), await, const/let, Promise.allSettled([...]), pragmas) and parse its cmd string with the shell parser.
2. Strip JavaScript comments (// and /* */, outside string literals) before extracting calls, so commented-out calls never count.

Use a small JS-aware tokenizer (string literals with escapes, comments), not regexes over the raw text. Keep the skill-load rules from guidance v1.

## Validator findings, run :8
```json
{
  "verdict": "fail",
  "candidate": "82fece319dfe29c9e58ba24d32f06dac7bdfd174",
  "context_digest": "6412cd29fde702019da854b93337a4135aac14e9c500d36ad9060fd42879f4ed",
  "checks": {
    "git diff --check": "PASS (exit 0; no output)",
    "git diff --check 9688435ee5eebe946f7145c358b33567b6601ef5 HEAD": "PASS (exit 0; no output)",
    "go test ./...": "PASS (exit 0; all packages passed)",
    "go test ./internal/trace -run TestSkill|TestClaudeNativeSkill -v": "PASS: boundary fixtures and native fixtures for claude_code, codex, pi, dot",
    "go test ./cmd/hev -run TestSkillsPreviewIsLocalAndCountsHarnesses -v": "PASS: each harness sessions=1 with_skills=1 would_gain=1; baseline replay zero gains",
    "go test ./internal/trace -run TestIndependentSkillProbe -v": "FAIL (exit 1; both subtests fail)"
  },
  "findings": [
    {
      "priority": "high",
      "location": "internal/trace/skills_shell.go:19",
      "issue": "Codex orchestration result wrapper misses an executed skill read",
      "input": "text(await tools.exec_command({cmd:\"cat /repo/skills/actual/SKILL.md\"}));",
      "expected": "one skill load: actual",
      "observed": "loads=[] want=1",
      "reason": "JS extraction only enters for await/const/let/pragma prefixes; text(...) falls through to the shell parser and is rejected. This is a valid Codex functions.exec command and the wrapper used in this validation session."
    },
    {
      "priority": "medium",
      "location": "internal/trace/skills_shell.go:21",
      "issue": "JavaScript comment is falsely counted as a skill read",
      "input": "await tools.exec_command({cmd:\"echo ready\"}); // tools.exec_command({cmd:\"cat /repo/skills/false/SKILL.md\"})",
      "expected": "zero skill loads",
      "observed": "loads=[{Name:false Path:/repo/skills/false/SKILL.md Source:/repo/skills Commit: TurnUUID: Seq:0 TS:}] want=0",
      "reason": "Regex scans command properties in JS comments without verifying an executable tools.exec_command call."
    }
  ],
  "reproduction": "Copy validation_probe_test.go.txt to internal/trace/validation_probe_test.go; run go test ./internal/trace -run TestIndependentSkillProbe -v; remove copied probe afterwards. Original probe removed after validation; candidate implementation unchanged.",
  "coverage": "Reviewed schema: skills filterable true, skill_loads false. Reviewed dashboard list/trace rendering and local filtering; supplied tests pass. No browser visual check performed. Fixtures are synthetic and no real historical corpus coverage count is claimed.",
  "scope": "No live writes, schema changes, credentials resolution, workers, push, merge, deploy, release, shared-service restart or acceptance. Findings-only commit preserves candidate implementation."
}
```

## Validator probe test (add as internal/trace/validation_probe_test.go)
```go
package trace
import "testing"
func TestIndependentSkillProbe(t *testing.T) {
 for _, tt := range []struct{name, command string; want int}{
 {"codex-result-wrapper", `text(await tools.exec_command({cmd:"cat /repo/skills/actual/SKILL.md"}));`, 1},
 {"javascript-comment", "await tools.exec_command({cmd:\"echo ready\"}); // tools.exec_command({cmd:\"cat /repo/skills/false/SKILL.md\"})", 0},
 } {
 t.Run(tt.name,func(t *testing.T){ turns:=CaptureSkills([]Turn{{Harness:"codex",Blocks:[]Block{{Type:"tool_use",ToolName:"exec",Text:"exec "+tt.command}}}}); got:=turns[0].Blocks[0].SkillLoads; if len(got)!=tt.want {t.Errorf("loads=%+v want=%d command=%s",got,tt.want,tt.command)} })
 }
}
```

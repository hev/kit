# Release checklist

Before tagging a release:

- Run `go test ./...`, `go vet ./...`, and `GOOS=windows go vet ./...`.
- Run one browser validation pass with `hev validate NAME` against a
  completed browser order. Inspect its verdict and acceptance evidence with
  `hev dispatch show NAME`; resolve failures before tagging.
- Build the candidate with its release version injected into
  `github.com/hev/kit/internal/version.Version`.
- In a temporary HOME with `.claude` and `.codex` directories, run the candidate's
  `hev skills sync` and `hev skills check`. Verify every skill is current.
- Review bundled skills for public content and shipped commands, then tag.

## Bundled skills

Previously only `hev-traces` was installed. The binary now embeds `hev-traces`,
`hev-dispatch`, `hev-board`, `hev-peers`, and `hev-handoff`. `hev up` and
`hev skills sync` install them into existing `~/.claude/skills` and
`~/.codex/skills` directories. Each has a `.kit-skill.json` sidecar with the kit
version and SHA-256 of SKILL.md. `hev s` and `hev skills check` report missing,
stale, locally edited, or current copies. Sync repairs missing and stale copies.
Unstamped copies matching historical kit content are stale and repaired. Unknown
unstamped copies are treated as local edits; inspect them before using
`hev skills sync --force`. Symlinks are preserved unless forced; forcing replaces
the link itself and does not write to its target.

Historical unstamped traces copies are recognized by the embedded
`skills/hev-traces/historical-sha256.txt` allowlist, derived from SHA-256 hashes
of `skills/hev-traces/SKILL.md` blobs in git history. Preserve those hashes when
updating the skill so upgrades can distinguish shipped copies from local edits.
`hev skills check` exits non-zero for any missing, stale, or locally edited copy.

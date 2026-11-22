package feed

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// RecordSchema versions spool records.
const RecordSchema = "hev.feed.record/v1"

// Thread kinds.
const (
	ThreadSession      = "session"
	ThreadRun          = "run"
	ThreadCron         = "cron"
	ThreadService      = "service"
	ThreadOrchestrator = "orchestrator"
	ThreadBoard        = "board"
	ThreadEvidence     = "evidence"
	ThreadPeers        = "peers"
)

// Thread says which Slack thread a record belongs in and what its parent
// line names.
type Thread struct {
	// Key identifies the thread. Runs and orchestrators use the attention
	// notice key (run:HOST/NAME, orchestrator:HOST/NAME), so their notices
	// land in the same thread as their turns.
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Name is the session, run or orchestrator name shown in the parent.
	Name    string `json:"name"`
	Harness string `json:"harness,omitempty"`
	Host    string `json:"host"`
	// Cwd is shown in the parent line; empty for client work.
	Cwd   string `json:"cwd,omitempty"`
	Model string `json:"model,omitempty"`
}

// Hook is the part of a hev.hook.event/v1 the feed keeps. A client
// session's record keeps no prompt, command or transcript.
type Hook struct {
	Event          string    `json:"event"`
	Harness        string    `json:"harness"`
	SessionID      string    `json:"session_id,omitempty"`
	Cwd            string    `json:"cwd,omitempty"`
	Model          string    `json:"model,omitempty"`
	Prompt         string    `json:"prompt,omitempty"`
	PromptBytes    int       `json:"prompt_bytes,omitempty"`
	Source         string    `json:"source,omitempty"`
	StopHookActive bool      `json:"stop_hook_active,omitempty"`
	TranscriptPath string    `json:"transcript_path,omitempty"`
	Time           time.Time `json:"time,omitzero"`
	Target         string    `json:"target,omitempty"`
	Tool           *struct {
		Name    string `json:"name"`
		Command string `json:"command,omitempty"`
	} `json:"tool,omitempty"`
}

// Record is one spooled feed event.
type Record struct {
	Schema string    `json:"schema"`
	ID     string    `json:"id"`
	Time   time.Time `json:"time"`
	Kind   string    `json:"kind"`
	// Client marks client work: the sender posts only a bare line and
	// never sends content to the model or uploads files.
	Client bool   `json:"client,omitempty"`
	Thread Thread `json:"thread"`
	Hook   *Hook  `json:"hook,omitempty"`
	// TranscriptSize is the transcript's size when the turn stopped; the
	// sender reads no further, so a later turn cannot leak into this one.
	TranscriptSize int64 `json:"transcript_size,omitempty"`
	// RunDir is the dispatch run or orchestrator directory (order.md,
	// rounds/).
	RunDir string `json:"run_dir,omitempty"`
	// Round is the orchestrator round a stop belongs to.
	Round int `json:"round,omitempty"`
	// Payload is the notice, board post or peer message JSON as handlers
	// received it.
	Payload json.RawMessage `json:"payload,omitempty"`
	// EvidenceDir holds an acceptance-evidence recording's result.json.
	EvidenceDir string `json:"evidence_dir,omitempty"`
}

// DirEnv overrides the feed's state directory.
const DirEnv = "HEV_FEED_DIR"

// OffEnv turns the feed off for one process tree (off, 0, false, no).
const OffEnv = "HEV_FEED"

// Dir is the feed's state: HEV_FEED_DIR or ~/.hev/feed.
func Dir() string {
	if d := os.Getenv(DirEnv); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "hev-feed")
	}
	return filepath.Join(home, ".hev", "feed")
}

// SpoolDir holds records waiting for the sender.
func SpoolDir(dir string) string { return filepath.Join(dir, "spool") }

// LockPath is held by the running sender.
func LockPath(dir string) string { return filepath.Join(dir, "sender.lock") }

// MaxSpool bounds the spool; the oldest records go first.
const MaxSpool = 2000

func off() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(OffEnv))) {
	case "off", "0", "false", "no":
		return true
	}
	// A test binary never reaches the operator's feed unless it set one up.
	return testing.Testing() && os.Getenv(DirEnv) == ""
}

// Add writes rec to the spool atomically.
func Add(dir string, rec Record) error {
	rec.Schema = RecordSchema
	if rec.Time.IsZero() {
		rec.Time = time.Now().UTC()
	}
	if rec.ID == "" {
		var b [6]byte
		_, _ = rand.Read(b[:])
		rec.ID = fmt.Sprintf("%019d-%s", rec.Time.UnixNano(), hex.EncodeToString(b[:]))
	}
	sp := SpoolDir(dir)
	if err := os.MkdirAll(sp, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	tmp := filepath.Join(sp, "."+rec.ID+".tmp")
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(sp, rec.ID+".json"))
}

// Pending lists spooled records oldest first, dropping the oldest past
// MaxSpool (Slack unreachable for a long time).
func Pending(dir string) []string {
	matches, _ := filepath.Glob(filepath.Join(SpoolDir(dir), "*.json"))
	sort.Strings(matches)
	if len(matches) > MaxSpool {
		for _, p := range matches[:len(matches)-MaxSpool] {
			_ = os.Remove(p)
		}
		matches = matches[len(matches)-MaxSpool:]
	}
	return matches
}

// SenderRunning reports whether a sender holds the lock.
func SenderRunning(dir string) bool {
	unlock, err := platformops.LockPath(LockPath(dir), true)
	if err != nil {
		return platformops.IsLockBusy(err)
	}
	unlock()
	return false
}

// Spawn starts `hev feed send` detached. Tests replace it.
var Spawn = func() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c := exec.Command(exe, "feed", "send")
	c.Stdin, c.Stdout, c.Stderr = nil, nil, nil
	c.Env = append(os.Environ(), "HEV_HOOKS=off")
	return startDetached(c)
}

// Kick wakes the sender when records wait and none is running.
func Kick(dir string) {
	if len(Pending(dir)) == 0 || SenderRunning(dir) {
		return
	}
	_ = Spawn()
}

// Wake restarts a stopped sender when records wait (hevd's scan cycle), so
// a paused feed retries without a new event.
func Wake() {
	if off() {
		return
	}
	Kick(Dir())
}

package compat

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/hev/kit/internal/platformops"
)

func uuidHex() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return hex.EncodeToString(b[:])
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// appendLine is the reference's append(): unlocked, no fsync.
func appendLine(path string, record *Obj) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err := f.WriteString(Dumps(record) + "\n"); err != nil {
		panic(err)
	}
}

// jsonLines yields every parseable line of a JSONL file (any JSON value).
func jsonLines(path string) []any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := []any{}
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			if v, e := Loads([]byte(line)); e == nil {
				out = append(out, v)
			}
		}
		if err != nil {
			break
		}
	}
	return out
}

// recent is the records of path whose ts falls within window seconds.
func recent(path string, window float64) []*Obj {
	out, cutoff := []*Obj{}, now()-window
	for _, v := range jsonLines(path) {
		rec, ok := v.(*Obj)
		if !ok {
			continue
		}
		ts, _ := rec.Num("ts")
		if !rec.Has("ts") {
			ts = 0
		}
		if ts >= cutoff {
			out = append(out, rec)
		}
	}
	return out
}

// writeJSON replaces path atomically, like the reference's write_json.
func writeJSON(path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.%d", path, os.Getpid())
	if err := os.WriteFile(tmp, []byte(Dumps(data)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readObj(path string) (*Obj, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadObj(b)
}

func (p *P) snapshot(host string) *Obj {
	value, err := readObj(filepath.Join(p.snapshots, host+".json"))
	if err != nil {
		return nil
	}
	// Keep explicitly offline rows out of cached reads and relay targets.
	kept := []any{}
	for _, r := range value.List("panes") {
		if o, ok := r.(*Obj); ok && participates(o.Str("role")) {
			kept = append(kept, o)
		}
	}
	value.Set("panes", kept)
	return value
}

func (p *P) snapshotList() []*Obj {
	out := []*Obj{}
	for _, path := range sortedGlob(filepath.Join(p.snapshots, "*.json")) {
		host := strings.TrimSuffix(filepath.Base(path), ".json")
		if snap := p.snapshot(host); snap != nil && host != p.selfLabel() {
			out = append(out, snap)
		}
	}
	return out
}

// --- inbox ----------------------------------------------------------------------

func (p *P) inboxOn(session string) bool {
	if p.managedRecord(session) != nil {
		return true
	}
	return strings.TrimSpace(p.tmuxQ("show-options", "-t", "="+session+":", "-qv", "@peers-inbox")) == "on"
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func (p *P) inboxPaths(session string) (string, string) {
	safe := unsafeName.ReplaceAllString(session, "_")
	return filepath.Join(p.inboxDir, safe+".jsonl"), filepath.Join(p.inboxDir, safe+".cursor")
}

func (p *P) inboxPath(session string) string { path, _ := p.inboxPaths(session); return path }

func (p *P) paneIncarnation(r *Row) string {
	if r.Managed != nil {
		return p.incarnation(r.Session, r.Managed.Str("incarnation"), true)
	}
	if strings.HasPrefix(r.ID, "pid:") {
		birth := 0.0
		if r.AgentPID != 0 {
			birth = p.started(r.AgentPID)
		}
		if birth == 0 {
			return ""
		}
		return fmt.Sprintf("process:%s:%d:%s", p.selfLabel(), r.AgentPID, FloatRepr(birth))
	}
	if r.TmuxIdentity != "" {
		return p.incarnation(r.Session, r.TmuxIdentity, true)
	}
	return p.incarnation(r.Session, "", false)
}

// incarnation is server lifetime + tmux's never-reused session ID. Never cache
// a name->identity mapping: a delayed sender must carry its binding.
func (p *P) incarnation(session, expected string, check bool) string {
	if managed := p.managedRecord(session); managed != nil {
		if !check || expected == managed.Str("incarnation") {
			return managed.Str("incarnation")
		}
		return ""
	}
	raw := strings.TrimSpace(p.tmuxQ("display-message", "-p", "-t", "="+session+":", "#{pid}:#{session_id}:#{session_created}"))
	if check && raw != expected {
		return ""
	}
	parts := strings.Split(raw, ":")
	if len(parts) != 3 || !isDigits(parts[0]) || !strings.HasPrefix(parts[1], "$") || !isDigits(parts[2]) {
		return ""
	}
	pid, _ := strconv.Atoi(parts[0])
	birth := p.started(pid)
	if birth == 0 {
		return ""
	}
	return sha256Hex(fmt.Sprintf("%s:%s:%s", p.selfLabel(), raw, FloatRepr(birth)))
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func lockFile(path string) func() {
	unlock, err := platformops.LockPath(path, false)
	if err != nil {
		panic(err)
	}
	return unlock
}

func (p *P) inboxLock(session string) func() {
	os.MkdirAll(p.inboxDir, 0o755)
	return lockFile(p.inboxPath(session) + ".lock")
}

// inboxRows retains corrupt/partial bytes for inspection, not obligations.
func inboxRows(path string) []*Obj {
	out := []*Obj{}
	for _, v := range jsonLines(path) {
		if o, ok := v.(*Obj); ok {
			out = append(out, o)
		}
	}
	return out
}

// inboxAppend: the caller holds the lock. Preserve a torn tail without joining rows.
func inboxAppend(path string, record *Obj) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	end, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		panic(err)
	}
	if end > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, end-1); err != nil {
			panic(err)
		}
		if last[0] != '\n' {
			if _, err := f.Write([]byte{'\n'}); err != nil {
				panic(err)
			}
		}
	}
	if _, err := f.Write([]byte(Dumps(record) + "\n")); err != nil {
		panic(err)
	}
	if err := f.Sync(); err != nil {
		panic(err)
	}
}

func isNumber(v any) bool {
	switch v.(type) {
	case int, int64, float64, bool:
		return true
	}
	return false
}

type inboxState struct {
	identity                  string
	messages, current, unread []*Obj
}

func (p *P) inboxView(session, identity string) inboxState {
	if identity == "" {
		identity = p.incarnation(session, "", false)
	}
	path := p.inboxPath(session)
	receipts := map[string]*Obj{}
	key := func(inc, id any) string { return Dumps([]any{inc, id}) }
	for _, event := range inboxRows(path + ".receipts.jsonl") {
		kind := event.Get("kind")
		if (Equal(kind, "consumed") || Equal(kind, "acknowledged")) && isNumber(event.Get("ts")) {
			k := key(event.Get("recipient_incarnation"), event.Get("id"))
			if receipts[k] == nil {
				receipts[k] = NewObj()
			}
			receipts[k].Set(kind.(string), event.Get("ts"))
		}
	}
	st := inboxState{identity: identity}
	for _, row := range inboxRows(path) {
		rec := receipts[key(row.Get("recipient_incarnation"), row.Get("id"))]
		transport := "queued"
		if rec.Has("acknowledged") {
			transport = "acknowledged"
		}
		m := row.Merge(NewObj("current", identity != "" && Equal(row.Get("recipient_incarnation"), identity),
			"consumed_at", rec.Get("consumed"), "acknowledged_at", rec.Get("acknowledged"), "transport_state", transport))
		st.messages = append(st.messages, m)
	}
	for _, m := range st.messages {
		if m.Get("current") == true {
			st.current = append(st.current, m)
			if m.Get("consumed_at") == nil {
				st.unread = append(st.unread, m)
			}
		}
	}
	return st
}

// hold appends once to a bound inbox. Consumption and ACK are separate receipts.
func (p *P) hold(session, header, message, sender string, extra *Obj) (*Obj, error) {
	if extra == nil {
		extra = NewObj()
	}
	extra = extra.Copy()
	role := strings.TrimSpace(p.tmuxQ("show-options", "-t", "="+session+":", "-qv", "@peers-role"))
	if role != "" && role != "foh" && role != "boh" {
		return nil, refuse("recipient is offline")
	}
	identity := p.incarnation(session, "", false)
	if identity == "" {
		return nil, refuse("cannot establish recipient incarnation for %s", session)
	}
	expected := any(identity)
	if extra.Has("recipient_incarnation") {
		expected = extra.Pop("recipient_incarnation")
	}
	if !Equal(expected, identity) {
		return nil, refuse("recipient incarnation changed; historical message was not delivered")
	}
	key := extra.Pop("id")
	if !Truthy(key) {
		key = uuidHex()
	}
	record := extra.Merge(NewObj("id", key, "ts", now(), "from", sender, "header", header, "message", message,
		"recipient_session", session, "recipient_incarnation", identity, "transport_state", "queued"))
	path := p.inboxPath(session)
	unlock := p.inboxLock(session)
	defer unlock()
	current := p.incarnation(session, "", false)
	if current == "" {
		return nil, refuse("cannot establish recipient incarnation before enqueue")
	}
	if current != identity {
		return nil, refuse("recipient incarnation changed before enqueue")
	}
	for _, old := range inboxRows(path) {
		if Equal(old.Get("id"), key) {
			for _, k := range []string{"recipient_incarnation", "from", "from_agent", "from_cwd", "message", "thread", "order", "run"} {
				if !Equal(old.Get(k), record.Get(k)) {
					return nil, refuse("message ID reused with different content or recipient")
				}
			}
			return old, nil
		}
	}
	inboxAppend(path, record)
	return record, nil
}

func (p *P) inboxSummary(session, expected string) *Obj {
	identity := p.incarnation(session, expected, expected != "")
	if identity == "" {
		return NewObj("recipient_incarnation", nil, "unread", nil, "unacknowledged", nil, "pending", []any{})
	}
	st := p.inboxView(session, identity)
	unacked := 0
	pending := []any{}
	for _, m := range st.current {
		if m.Get("acknowledged_at") == nil {
			unacked++
			from := m.Get("from")
			if !m.Has("from") {
				from = "?"
			}
			pending = append(pending, NewObj("id", m.Get("id"), "from", from, "consumed", m.Get("consumed_at") != nil))
		}
	}
	return NewObj("recipient_incarnation", st.identity, "unread", len(st.unread), "unacknowledged", unacked, "pending", pending)
}

var tokenRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// runtimeRecipient: a launcher capability delegates receipt ownership to
// detached runtime tools. Same-user capability, never a sender override.
func (p *P) runtimeRecipient(rows []*Row) *Row {
	token := os.Getenv("PEERS_INBOX_TOKEN")
	if !tokenRE.MatchString(token) {
		return nil
	}
	binding, err := readObj(filepath.Join(p.stateDir, "runtime", token+".json"))
	if err != nil {
		return nil
	}
	for _, k := range []string{"owner_pid", "owner_started", "session", "pane", "incarnation"} {
		if !binding.Has(k) {
			return nil
		}
	}
	pid, ok := intOf(binding.Get("owner_pid"))
	if !ok || !Equal(p.started(pid), binding.Get("owner_started")) {
		return nil
	}
	for _, r := range rows {
		if Equal(r.Session, binding.Get("session")) && Equal(r.ID, binding.Get("pane")) {
			if !participates(r.Role) || !Equal(p.paneIncarnation(r), binding.Get("incarnation")) {
				return nil
			}
			return r
		}
	}
	return nil
}

// withoutFrom runs f with PEERS_FROM removed: receipt ownership cannot be
// borrowed with the sender's --from override.
func withoutFrom[T any](f func() T) T {
	named, ok := os.LookupEnv("PEERS_FROM")
	os.Unsetenv("PEERS_FROM")
	defer func() {
		if ok {
			os.Setenv("PEERS_FROM", named)
		}
	}()
	return f()
}

func (p *P) bindRuntime(rows []*Row, args []string) {
	// Only an actual local launcher can create a delegation.
	mine := withoutFrom(func() *Row { return p.me(rows) })
	if mine == nil || mine.Headless || !participates(mine.Role) || !p.inboxOn(mine.Session) {
		p.die("runtime binding requires the actual registered interactive recipient")
	}
	pid := 0
	var err error = fmt.Errorf("missing")
	if len(args) > 0 {
		pid, err = strconv.Atoi(strings.TrimSpace(args[0]))
	}
	if err != nil {
		p.die("inbox bind-runtime LAUNCHER_PID")
	}
	if !intIn(p.ps().ancestors(os.Getpid()), pid) {
		p.die("runtime binding requires launcher ancestry")
	}
	birth := p.started(pid)
	identity := p.paneIncarnation(mine)
	if birth == 0 || identity == "" {
		p.die("runtime binding requires a live launcher and recipient incarnation")
	}
	token := uuidHex() + uuidHex()
	root := filepath.Join(p.stateDir, "runtime")
	os.MkdirAll(root, 0o700)
	f, err := os.OpenFile(filepath.Join(root, token+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		panic(err)
	}
	f.WriteString(Dumps(NewObj("session", mine.Session, "pane", mine.ID, "incarnation", identity,
		"owner_pid", pid, "owner_started", birth)))
	f.Sync()
	f.Close()
	fmt.Fprintln(p.out, token)
}

func (p *P) recipientMe(rows []*Row) *Row {
	if _, ok := os.LookupEnv("PEERS_INBOX_TOKEN"); ok {
		return p.runtimeRecipient(rows)
	}
	return withoutFrom(func() *Row { return p.me(rows) })
}

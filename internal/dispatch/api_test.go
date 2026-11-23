package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAPILogBoundsAndRedactsOnlyCompleteRecords(t *testing.T) {
	dir := t.TempDir()
	secret := "v4.public." + strings.Repeat("Abc0", 20)
	first := "{\"message\":\"" + secret + " " + dir + "\"}\n"
	last := "{\"message\":\"visible\"}\n"
	oversized := strings.Repeat("a", 700) + secret + "\n"
	if err := os.WriteFile(filepath.Join(dir, "log"), []byte(first+oversized+last), 0600); err != nil {
		t.Fatal(err)
	}
	q := Request{APILogLimit: 512}
	var result strings.Builder
	for count := 0; count < 10; count++ {
		out, offset, active, err := readAPILog(dir, q, false)
		if err != nil || len(out) > q.APILogLimit || offset <= q.Offset {
			t.Fatalf("invalid bounded read at %d: %d bytes, next %d, error %v", q.Offset, len(out), offset, err)
		}
		result.WriteString(out)
		q.Offset = offset
		if !active {
			break
		}
	}
	if got := result.String(); strings.Contains(got, secret) || strings.Contains(got, dir) || strings.Contains(got, strings.Repeat("a", 30)) ||
		!strings.Contains(got, "REDACTED") || !strings.HasSuffix(got, last) {
		t.Fatal("log released a secret, a local path, a partial record, or lost a complete record")
	}
}

func TestAPILogRoundAndRotatedOffset(t *testing.T) {
	dir := t.TempDir()
	const line = "round output\n"
	if err := os.WriteFile(filepath.Join(dir, "round-3.log"), []byte(line), 0600); err != nil {
		t.Fatal(err)
	}
	out, offset, active, err := readAPILog(dir, Request{Round: 3, APILogLimit: 512, Offset: 500}, false)
	if err != nil || out != line || offset != int64(len(line)) || active {
		t.Fatalf("round/rotation read: %q, %d, %t, %v", out, offset, active, err)
	}
	_, _, _, err = readAPILog(dir, Request{Round: 4, APILogLimit: 512}, false)
	if !os.IsNotExist(err) {
		t.Fatalf("missing round: %v", err)
	}
}

func TestQueueAPICancelKeepsHaltedRecord(t *testing.T) {
	queueTestHome(t)
	q := queueRequest("cancelled-api")
	q.APILocalOnly = true
	if _, err := enqueue(q); err != nil {
		t.Fatal(err)
	}
	_, found, err := controlQueued(Request{Action: "stop", Name: q.Name, APILocalOnly: true})
	if !found || err != nil {
		t.Fatalf("queued stop: %t, %v", found, err)
	}
	entries, err := queueEntries()
	if err != nil || len(entries) != 0 {
		t.Fatalf("cancelled run remained queued: %d, %v", len(entries), err)
	}
	var result Result
	if err := readJSON(filepath.Join(Root(), q.Name, "result.json"), &result); err != nil || State(&result, false) != "halted" {
		t.Fatalf("cancelled run is not readable as halted: %v", err)
	}
}

func TestAPIPurgeKeepsBranchAndRefusesDirtyWorktree(t *testing.T) {
	queueTestHome(t)
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if _, err := command(repo, append([]string{"git"}, args...)...); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "--initial-branch=main")
	git("-c", "user.name=Example", "-c", "user.email=example@example.com", "commit", "--allow-empty", "-m", "Initial")
	dir := filepath.Join(Root(), "example")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(dir, "wt")
	git("worktree", "add", "-b", "d/example", wt)
	if err := WriteJSON(filepath.Join(dir, "meta.json"), Meta{Order: Order{Name: "example", Repo: repo}, Branch: "d/example", ResolvedRepo: repo, BranchCreated: true, WorktreeCreated: true}); err != nil {
		t.Fatal(err)
	}
	if err := WriteJSON(filepath.Join(dir, "result.json"), Result{End: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	dirty := filepath.Join(wt, "untracked")
	if err := os.WriteFile(dirty, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	purge := Request{Action: "clean", Name: "example", APILocalOnly: true}
	if _, err := Handle(purge); err == nil {
		t.Fatal("purge removed a dirty worktree")
	}
	if _, err := os.Stat(filepath.Join(dir, "result.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	if _, err := Handle(purge); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("purge left the run directory: %v", err)
	}
	git("show-ref", "--verify", "refs/heads/d/example")
	git("branch", "-d", "d/example") // Also proves worktree registration was removed.
}

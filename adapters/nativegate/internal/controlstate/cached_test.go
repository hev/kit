package controlstate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const fakeValue = "fake-db-credential-never-log"

// fakeTools writes a fake op and, optionally, a fake hev that exits with
// hevExit (0 answers JSON for every NAME=ref it was given). Each records one
// line per run; neither contacts 1Password.
func fakeTools(t *testing.T, hevExit string) (bin, calls string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fakes are shell scripts")
	}
	dir := t.TempDir()
	bin = filepath.Join(dir, "bin")
	os.Mkdir(bin, 0o700)
	calls = filepath.Join(dir, "calls")
	op := "#!/bin/sh\necho \"op $1\" >> \"$FAKE_CALLS\"\nprintf '%s' '" + fakeValue + "'\n"
	os.WriteFile(filepath.Join(bin, "op"), []byte(op), 0o700)
	if hevExit != "" {
		hev := `#!/bin/sh
echo "hev $2 $HEV_OP_TOKEN_FILE" >> "$FAKE_CALLS"
[ "` + hevExit + `" = 0 ] || exit ` + hevExit + `
out='{'; sep=''
for a in "$@"; do
  case "$a" in NATIVE_GATE_REF_*=op://*) out="$out$sep\"${a%%=*}\":\"` + fakeValue + `\""; sep=',';; esac
done
printf '%s}\n' "$out"
`
		os.WriteFile(filepath.Join(bin, "hev"), []byte(hev), 0o700)
	}
	t.Setenv("FAKE_CALLS", calls)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("HEV_SECRETS_READ_LOG", filepath.Join(dir, "op-reads.log"))
	return bin, calls
}

func lines(t *testing.T, p string) []string {
	b, _ := os.ReadFile(p)
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

var gateRefs = []string{"op://example/db/host", "op://example/db/user", "op://example/db/password"}

func TestCachedResolverOneBatchThroughDaemon(t *testing.T) {
	_, calls := fakeTools(t, "0")
	v, err := CachedResolver{}.ResolveAll(context.Background(), gateRefs)
	if err != nil || len(v) != 3 || v[2] != fakeValue {
		t.Fatalf("resolution failed: %v", err)
	}
	if got := lines(t, calls); len(got) != 1 || !strings.HasPrefix(got[0], "hev env") {
		t.Fatalf("want one hev batch and no op call, got %q", got)
	}
}

func TestCachedResolverFallsBackWhenNoDaemon(t *testing.T) {
	for _, exit := range []string{"69", "1"} { // 1: an older hev without `secrets env`
		_, calls := fakeTools(t, exit)
		v, err := CachedResolver{}.ResolveAll(context.Background(), gateRefs)
		if err != nil || v[0] != fakeValue {
			t.Fatalf("exit %s: fallback failed: %v", exit, err)
		}
		if got := lines(t, calls); len(got) != 4 || got[1] != "op read" {
			t.Fatalf("exit %s: want hev then one op read per ref, got %q", exit, got)
		}
		log, _ := os.ReadFile(os.Getenv("HEV_SECRETS_READ_LOG"))
		if strings.Count(string(log), "caller=native-gate_direct") != 3 || strings.Contains(string(log), fakeValue) || strings.Contains(string(log), "op://") {
			t.Fatal("direct reads must be logged once each, without values or references")
		}
	}
	_, calls := fakeTools(t, "") // no hev at all
	if _, err := (CachedResolver{}).ResolveAll(context.Background(), gateRefs); err != nil {
		t.Fatal(err)
	}
	if got := lines(t, calls); len(got) != 3 {
		t.Fatalf("without hev want three op reads as before, got %q", got)
	}
}

func TestCachedResolverDaemonRefusalIsFinal(t *testing.T) {
	_, calls := fakeTools(t, "75")
	if _, err := (CachedResolver{}).ResolveAll(context.Background(), gateRefs); err == nil || strings.Contains(err.Error(), fakeValue) {
		t.Fatal("a daemon refusal (rate limit) must fail without a value")
	}
	if got := lines(t, calls); len(got) != 1 {
		t.Fatalf("a refusal must not fall back to op, got %q", got)
	}
}

func TestCachedResolverPassesTokenFileByPath(t *testing.T) {
	_, calls := fakeTools(t, "0")
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "")
	r := CachedResolver{Direct: OPResolver{TokenFile: "/run/example/token"}}
	if _, err := r.ResolveAll(context.Background(), gateRefs[:1]); err != nil {
		t.Fatal(err)
	}
	// Only the token file's path crosses to hev, in its environment.
	if got := lines(t, calls); len(got) != 1 || got[0] != "hev env /run/example/token" {
		t.Fatalf("got %q", got)
	}
}

package controlstate

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestCachedResolverRefusesUnguardedFallback(t *testing.T) {
	for _, exit := range []string{"69", "1", ""} {
		_, calls := fakeTools(t, exit)
		if _, err := (CachedResolver{}).ResolveAll(context.Background(), gateRefs); err == nil {
			t.Fatal("unguarded fallback succeeded")
		}
		for _, line := range lines(t, calls) {
			if strings.HasPrefix(line, "op ") {
				t.Fatal("bypassed host guard")
			}
		}
	}
}

func TestCachedResolverDaemonRefusalIsFinal(t *testing.T) {
	_, calls := fakeTools(t, "76")
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

// After a rate-limit answer the gate's direct path makes no request until the
// reset, across fresh resolvers (a restarted gate), and the cooldown file it
// shares with kit holds no value, token or reference.
func TestDirectRateLimitPersistsHostCooldown(t *testing.T) {
	bin, calls := fakeTools(t, "")
	limited := "#!/bin/sh\necho \"op $1\" >> \"$FAKE_CALLS\"\n[ -n \"$FAKE_LIMIT\" ] && { echo \"[ERROR] Too many requests. daily rate limit exceeded, reset at $FAKE_LIMIT\" >&2; exit 1; }\nprintf '%s' '" + fakeValue + "'\n"
	os.WriteFile(filepath.Join(bin, "op"), []byte(limited), 0o700)
	t.Setenv("FAKE_LIMIT", time.Now().Add(2*time.Hour).UTC().Format(time.RFC3339))
	for i := 0; i < 5; i++ {
		if _, err := (OPResolver{}).Resolve(context.Background(), gateRefs[0]); err == nil {
			t.Fatal("want a failure during the cooldown")
		}
	}
	if got := lines(t, calls); len(got) != 1 {
		t.Fatalf("five gate starts inside the cooldown made %d op requests, want 1", len(got))
	}
	b, err := os.ReadFile(hostCooldownPath())
	if err != nil || strings.Contains(string(b), fakeValue) || strings.Contains(string(b), "op://") || !strings.Contains(string(b), "daily rate limit") {
		t.Fatalf("cooldown file missing or unsafe: %v", err)
	}
	// Past the reset: one read goes through and the first success clears it.
	os.WriteFile(hostCooldownPath(), []byte(`{"limit":"daily rate limit","reset":"`+time.Now().Add(-time.Second).UTC().Format(time.RFC3339)+`"}`), 0o600)
	t.Setenv("FAKE_LIMIT", "")
	if _, err := (OPResolver{}).Resolve(context.Background(), gateRefs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(hostCooldownPath()); !os.IsNotExist(err) {
		t.Fatal("first success after the reset must clear the cooldown")
	}
}

// A Key Vault reference goes through hev like a 1Password one. With no daemon
// it is read by hev directly (no --cache-only), never by op; without hev it
// fails rather than reaching op.
func TestCachedResolverKeyVaultReferences(t *testing.T) {
	bin, calls := fakeTools(t, "")
	hev := `#!/bin/sh
cache=no
for a in "$@"; do [ "$a" = --cache-only ] && cache=yes; done
echo "hev $2 cache-only=$cache" >> "$FAKE_CALLS"
[ "$cache" = yes ] && exit 69
out='{'; sep=''
for a in "$@"; do
  case "$a" in NATIVE_GATE_REF_*=op://*|NATIVE_GATE_REF_*=azkv://*) out="$out$sep\"${a%%=*}\":\"` + fakeValue + `\""; sep=',';; esac
done
printf '%s}\n' "$out"
`
	os.WriteFile(filepath.Join(bin, "hev"), []byte(hev), 0o700)
	refs := []string{"op://example/db/host", "azkv://example-vault/db-user", "azkv://example-vault.vault.usgovcloudapi.net/db-password/0123456789abcdef0123456789abcdef"}
	v, err := CachedResolver{}.ResolveAll(context.Background(), refs)
	if err != nil || len(v) != 3 || v[1] != fakeValue || v[2] != fakeValue {
		t.Fatalf("resolution failed: %v", err)
	}
	if got := lines(t, calls); len(got) != 2 || got[0] != "hev env cache-only=yes" || got[1] != "hev env cache-only=no" {
		t.Fatalf("want the daemon asked, then hev's direct read, and no op: %q", got)
	}
	os.Remove(filepath.Join(bin, "hev"))
	if _, err := (CachedResolver{}).ResolveAll(context.Background(), refs); err == nil || !strings.Contains(err.Error(), "need hev") {
		t.Fatalf("without hev a Key Vault reference must fail: %v", err)
	}
	if got := lines(t, calls); len(got) != 2 {
		t.Fatalf("op must never see a Key Vault reference: %q", got)
	}
	for _, bad := range []string{"azkv://", "azkv://ab/x", "azkv://example-vault/", "azkv://example-vault/a/b", "azkv://example-vault/a?x=1", "azkv://ex--ample/a", "azkv://example-vault/a\n"} {
		if validRef(bad) {
			t.Errorf("accepted malformed %q", bad)
		}
	}
}

// A secret:// reference goes through hev like a Key Vault one: hev maps it
// through the host's secret manager. With no daemon hev reads it directly;
// without hev it fails and never reaches op. Malformed names are refused.
func TestCachedResolverNeutralReferences(t *testing.T) {
	bin, calls := fakeTools(t, "")
	hev := `#!/bin/sh
cache=no
for a in "$@"; do [ "$a" = --cache-only ] && cache=yes; done
echo "hev $2 cache-only=$cache" >> "$FAKE_CALLS"
[ "$cache" = yes ] && exit 69
out='{'; sep=''
for a in "$@"; do
  case "$a" in NATIVE_GATE_REF_*=secret://*|NATIVE_GATE_REF_*=op://*) out="$out$sep\"${a%%=*}\":\"` + fakeValue + `\""; sep=',';; esac
done
printf '%s}\n' "$out"
`
	os.WriteFile(filepath.Join(bin, "hev"), []byte(hev), 0o700)
	refs := []string{"op://example/db/host", "secret://db-user", "secret://db-password"}
	v, err := CachedResolver{}.ResolveAll(context.Background(), refs)
	if err != nil || len(v) != 3 || v[1] != fakeValue || v[2] != fakeValue {
		t.Fatalf("resolution failed: %v", err)
	}
	if got := lines(t, calls); len(got) != 2 || got[0] != "hev env cache-only=yes" || got[1] != "hev env cache-only=no" {
		t.Fatalf("want the daemon asked, then hev's direct read, and no op: %q", got)
	}
	os.Remove(filepath.Join(bin, "hev"))
	if _, err := (CachedResolver{}).ResolveAll(context.Background(), refs); err == nil || !strings.Contains(err.Error(), "need hev") {
		t.Fatalf("without hev a secret:// reference must fail: %v", err)
	}
	if _, err := (OPResolver{}).Resolve(context.Background(), "secret://db-user"); err == nil || !strings.Contains(err.Error(), "need hev") {
		t.Fatalf("op must refuse a secret:// reference: %v", err)
	}
	if got := lines(t, calls); len(got) != 2 {
		t.Fatalf("op must never see a secret:// reference: %q", got)
	}
	for _, bad := range []string{"secret://", "secret://DB", "secret://db_user", "secret://db/user", "secret://db user", "secret://db\n"} {
		if validRef(bad) {
			t.Errorf("accepted malformed %q", bad)
		}
	}
}

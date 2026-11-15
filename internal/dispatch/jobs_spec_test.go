package dispatch

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestJobValidation(t *testing.T) {
	host, _ := os.Hostname()
	cwd := t.TempDir()
	valid := fmt.Sprintf("---\nkind: job\nname: sync\nhost: %s\nexec:\n  argv: [sh, -c, 'echo stub']\n  cwd: %s\nschedule:\n  every: 1m\ntimeout: 1d\nreply: operator\n---\n", host, cwd)
	s, e := ParseJob([]byte(valid))
	if e != nil {
		t.Fatal(e)
	}
	if s.Overlap != "skip" || s.Schedule.CatchUp != "none" {
		t.Fatal("defaults not applied")
	}
	tests := []struct{ name, old, new string }{
		{"kind", "kind: job", "kind: order"},
		{"missing-name", "name: sync\n", ""},
		{"name-leading-digit", "name: sync", "name: 1sync"},
		{"name-length", "name: sync", "name: " + strings.Repeat("a", 64)},
		{"missing-host", "host: " + host + "\n", ""},
		{"empty-argv", "argv: [sh, -c, 'echo stub']", "argv: []"},
		{"empty-executable", "argv: [sh, -c, 'echo stub']", "argv: ['', arg]"},
		{"argv-type", "argv: [sh, -c, 'echo stub']", "argv: [42]"},
		{"relative-cwd", cwd, "relative"},
		{"missing-cwd", cwd, cwd + "/does-not-exist"},
		{"short-interval", "every: 1m", "every: 59s"},
		{"zero-interval", "every: 1m", "every: 0m"},
		{"negative-interval", "every: 1m", "every: -1m"},
		{"overflow-interval", "every: 1m", "every: 9999999999999999999d"},
		{"calendar", "every: 1m", "calendar: '* * * * *'"},
		{"timezone", "every: 1m", "every: 1m\n  timezone: Etc/UTC"},
		{"catch-up", "every: 1m", "every: 1m\n  catch_up: all"},
		{"unknown-root", "kind: job", "kind: job\nextra: true"},
		{"unknown-exec", "cwd: " + cwd, "cwd: " + cwd + "\n  env: {}"},
		{"unknown-retry", "reply: operator", "retry: {delay: 1m}\nreply: operator"},
		{"order-target", "kind: job", "kind: job\norder: /tmp/order.md"},
		{"timeout-missing", "timeout: 1d\n", ""},
		{"timeout-zero", "timeout: 1d", "timeout: 0s"},
		{"timeout-unlimited", "timeout: 1d", "timeout: never"},
		{"timeout-max", "timeout: 1d", "timeout: 24h1s"},
		{"overlap", "reply: operator", "overlap: queue-one\nreply: operator"},
		{"retry-nonzero", "reply: operator", "retry: {attempts: 1}\nreply: operator"},
		{"retry-negative", "reply: operator", "retry: {attempts: -1}\nreply: operator"},
		{"retry-type", "reply: operator", "retry: {attempts: '0'}\nreply: operator"},
		{"reply-missing", "reply: operator\n", ""},
		{"secret-literal", "reply: operator", "secrets: {TOKEN: literal-value}\nreply: operator"},
		{"secret-provider-uri", "reply: operator", "secrets: {TOKEN: 'op://vault/item/password'}\nreply: operator"},
		{"secret-invalid-name", "reply: operator", "secrets: {TOKEN: 'secret://UPPER'}\nreply: operator"},
		{"secret-env-invalid", "reply: operator", "secrets: {'INVALID-NAME': 'secret://token'}\nreply: operator"},
		{"secret-job-reserved", "reply: operator", "secrets: {HEV_JOB_NAME: 'secret://token'}\nreply: operator"},
		{"secret-bootstrap", "reply: operator", "secrets: {OP_SERVICE_ACCOUNT_TOKEN: 'secret://token'}\nreply: operator"},
		{"secret-cache", "reply: operator", "secrets: {HEV_SECRETS_CACHE: 'secret://token'}\nreply: operator"},
		{"secret-path", "reply: operator", "secrets: {PATH: 'secret://token'}\nreply: operator"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, e := ParseJob([]byte(strings.Replace(valid, tt.old, tt.new, 1))); e == nil {
				t.Fatal("accepted invalid spec")
			}
		})
	}
}
func TestJobDurations(t *testing.T) {
	d, e := JobDuration("1d2h3m4s")
	if e != nil || d != 26*time.Hour+3*time.Minute+4*time.Second {
		t.Fatal(d, e)
	}
	for _, s := range []string{"0", "0s", "1week", "1hgarbage", "-1m", "1h 2m"} {
		if _, e := JobDuration(s); e == nil {
			t.Fatal("accepted", s)
		}
	}
}
func TestJobEmptySecretsApply(t *testing.T) {
	jobHome(t)
	s := jobFixture(t)
	s.Secrets = nil
	if _, e := ApplyJob(s, time.Now(), false, true); e != nil {
		t.Fatal("empty optional secrets must remain valid", e)
	}
}

package main

import (
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets"
)

// hev secrets status tells "no endpoint" from "a daemon is up but does not
// serve this caller", and names the settings, never a value.
func TestSecretsStatusSaysWhyTheDaemonIsUnavailable(t *testing.T) {
	ep := "/tmp/hev-test.sock"
	t.Setenv(secrets.EndpointEnv, ep)
	cases := []struct {
		reason string
		want   []string
	}{
		{secrets.UnavailableNoEndpoint, []string{"daemon:          unavailable (no local endpoint at " + ep + "; reads go direct)"}},
		{secrets.UnavailableNoCredential, []string{
			"daemon:          up at " + ep + ", but not for this caller: it has no 1Password credential",
			"OP_SERVICE_ACCOUNT_TOKEN", "HEV_OP_TOKEN_FILE", "HEV_OP_DPAPI_FILE", "HEV_OP_DPAPI_SCOPE", "HEV_OP_DPAPI_ENTROPY",
		}},
		{secrets.UnavailableOtherAccount, []string{
			"daemon:          up at " + ep + ", but not for this caller: its 1Password credential is for a different account than the daemon's",
			"OP_SERVICE_ACCOUNT_TOKEN", "HEV_OP_TOKEN_FILE", "HEV_OP_DPAPI_FILE",
		}},
	}
	for _, c := range cases {
		var b strings.Builder
		s := &secrets.Status{DaemonUnavailable: c.reason}
		if c.reason != secrets.UnavailableNoEndpoint {
			s.DaemonUnavailableProviders = []string{"op"}
		}
		writeSecretsStatus(&b, s)
		out := b.String()
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: missing %q in\n%s", c.reason, w, out)
			}
		}
		if c.reason != secrets.UnavailableNoEndpoint && strings.Contains(out, "no local endpoint") {
			t.Errorf("%s: still says no local endpoint:\n%s", c.reason, out)
		}
		t.Logf("%s:\n%s", c.reason, strings.SplitN(out, "host read log", 2)[0])
	}
}

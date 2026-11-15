package storagemode

import (
	"os"
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets/fakeop"
)

func TestConfigReferenceAndPrecedence(t *testing.T) {
	for _, source := range []string{"config-ref", "HEVLAYER_API_KEY", "HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF", "config-key"} {
		t.Run(source, func(t *testing.T) {
			config := isolate(t)
			fakeop.Install(t)
			t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-test-token")
			text := "[layer]\napi_key_ref = 'op://test/layer/key'\n"
			want, label := fakeop.Value, "config [layer].api_key_ref"
			switch source {
			case "HEVLAYER_API_KEY":
				t.Setenv(source, "env-key")
				want, label = "env-key", source
			case "HEVLAYER_API_KEY_REF", "HEVLAYER_API_KEY_OP_REF":
				t.Setenv(source, "op://test/env/key")
				label = source
				// An invalid lower-precedence reference must never be read.
				text = "[layer]\napi_key_ref = 'invalid://reference'\n"
			case "config-key":
				text += "api_key = 'config-key'\n"
				want, label = "config-key", "config [layer].api_key"
			}
			if err := os.WriteFile(config, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			r, err := Resolve(Options{ConfigKey: true})
			if err != nil || r.Mode != Layer || r.Key != want || r.Source != label {
				t.Fatalf("mode=%s source=%s err=%v", r.Mode, r.Source, err)
			}
			if source == "config-ref" {
				// Board and other callers that ignore literal TOML keys still use the
				// shared credential reference.
				t.Setenv("HEV_SECRETS_CALLER", "config-ref-board")
				all, e := Resolve(Options{})
				if e != nil || all.Key != want || all.Source != label {
					t.Fatalf("shared reference: mode=%s source=%s err=%v", all.Mode, all.Source, e)
				}
			}
			peek, err := Resolve(Options{ConfigKey: true, Peek: true})
			if err != nil || peek.Mode != Layer || peek.Key != "" || peek.Source != label {
				t.Fatalf("peek: %+v %v", peek, err)
			}
		})
	}
}

func TestUnreadableConfigReferenceNeverFallsBackToFiles(t *testing.T) {
	config := isolate(t)
	fakeop.Install(t)
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-test-token")
	t.Setenv("FAKE_OP_ERROR", "not allowed")
	os.WriteFile(config, []byte("[layer]\napi_key_ref = 'op://test/layer/key'\n"), 0600)
	r, err := Resolve(Options{ConfigKey: true})
	if err == nil || !strings.Contains(err.Error(), "configured but could not be read") || r.Mode == Filesystem {
		t.Fatalf("mode=%s err=%v", r.Mode, err)
	}
}

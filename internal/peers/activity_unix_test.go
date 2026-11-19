//go:build !windows

package peers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPeerSendActivityHook(t *testing.T) {
	const body = "token ghp_AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	for _, mode := range []string{"capture", "off", "disabled", "client", "hang", "fail", "rejected", "ssh"} {
		t.Run(mode, func(t *testing.T) {
			e, call := fixture(t)
			dir := t.TempDir()
			capture := filepath.Join(dir, "capture")
			t.Setenv("PATH", dir)
			t.Setenv("HEV_HOOKS", "")
			t.Setenv("HOOK_CAPTURE", capture)
			t.Setenv("HEV_PEERS_STEER_FALLBACK", "")
			t.Setenv("HEV_CONFIG", filepath.Join(dir, "config.toml"))
			config := "[hooks]\ntimeout = \"150ms\"\n"
			script := "#!/bin/sh\nprintf '%s %s %s\\n' \"$1\" \"$HEV_HOOK_EVENT\" \"$HEV_HOOK_HARNESS\" > \"$HOOK_CAPTURE\"\n/bin/cat >> \"$HOOK_CAPTURE\"\n"
			target, wantTo := "desk", "local/desk"
			switch mode {
			case "off":
				t.Setenv("HEV_HOOKS", "off")
			case "disabled":
				config += "enabled = false\n"
			case "client":
				config += "[notify]\nclient = true\n"
			case "hang":
				script = "#!/bin/sh\nexec /bin/sleep 5\n"
			case "fail":
				script = "#!/bin/sh\nexit 23\n"
			case "rejected":
				target = "missing:0.0"
			case "ssh":
				e.Transport = &recordingTransport{}
				target, wantTo = "remote/desk", "remote/desk"
			}
			for path, content := range map[string]string{filepath.Join(dir, "hev-hook-capture"): script, os.Getenv("HEV_CONFIG"): config} {
				if err := os.WriteFile(path, []byte(content), 0700); err != nil {
					t.Fatal(err)
				}
			}
			start := time.Now()
			out, err := call("", "send", target, body, "--from", "sender", "--id", "stable", "--reply-to", "parent")
			if (err != nil) != (mode == "rejected") || time.Since(start) > time.Second {
				t.Fatalf("send: %q, %v after %s", out, err, time.Since(start))
			}
			hookDir := filepath.Join(os.Getenv("HOME"), ".hev", "hooks")
			logPath := filepath.Join(hookDir, "events.jsonl")
			if mode == "off" || mode == "disabled" || mode == "rejected" {
				for _, path := range []string{capture, logPath} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("unexpected hook output %s: %v", path, err)
					}
				}
				return
			}
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			record := decode(t, string(raw))
			if _, ok := record["text"]; ok || record["text_bytes"] != float64(len(body)) {
				t.Fatalf("unexpected hook log: %s", raw)
			}
			if mode == "hang" || mode == "fail" {
				warning, err := os.ReadFile(filepath.Join(hookDir, "warnings.log"))
				want := map[string]string{"hang": "timed out", "fail": "exit status 23"}[mode]
				if err != nil || !strings.Contains(string(warning), want) {
					t.Fatalf("warning: %s, %v", warning, err)
				}
				return
			}
			raw, err = os.ReadFile(capture)
			if err != nil {
				t.Fatal(err)
			}
			header, payload, _ := strings.Cut(string(raw), "\n")
			ev := decode(t, payload)
			if header != "peer-message peer-message kit" || ev["schema"] != "hev.peer.message/v1" ||
				ev["event"] != "peer-message" || ev["id"] != "stable" || ev["from"] != "sender" ||
				ev["to"] != wantTo || ev["host"] != "local" || ev["reply_to"] != "parent" ||
				ev["kind"] != "note" || ev["chat"] != false || ev["text_bytes"] != float64(len(body)) {
				t.Fatalf("unexpected hook capture: %s", raw)
			}
			if mode == "client" {
				if _, ok := ev["text"]; ok {
					t.Fatalf("client content reached the handler: %s", raw)
				}
			} else if text, ok := ev["text"].(string); !ok || !strings.HasPrefix(text, "token ") || strings.Contains(text, "AbCdEfGh") {
				t.Fatalf("missing or unredacted text: %s", raw)
			}
		})
	}
}

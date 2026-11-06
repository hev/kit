package intake

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const previewTemplate = "---\nname: {{name}}\nrepo: example\n---\n{{identifier}} {{url}} {{title}} {{status}}\n## Acceptance\n- Done\n"

func previewFixture(t *testing.T, template string) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	tpl := filepath.Join(root, "order.md")
	config := filepath.Join(root, "intake.toml")
	state := filepath.Join(root, "state")
	if err := os.WriteFile(tpl, []byte(template), 0600); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("state_dir = %q\n[linear]\napi_key_env = 'PREVIEW_TEST_KEY'\nteam = 'TEST'\nassignee = 'bot'\nallowed_actor_team = 'TEST'\n[[status]]\nname = 'Ready'\naction = 'fix'\nmode = 'live'\nhost = 'worker'\nconcurrency = 'code'\ntemplate = %q\n", state, tpl)
	if err := os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return config, state, tpl
}
func previewArgs(config, title string) []string {
	return []string{"linear", "render", "--config", config, "--status", "Ready", "--identifier", "TEST-1", "--url", "https://example.invalid/TEST-1", "--title", title}
}
func TestRenderValidation(t *testing.T) {
	for _, tc := range []struct {
		name, template, title, message string
		code                           int
	}{
		{"valid", previewTemplate, "A title", "", 0},
		{"acceptance", strings.ReplaceAll(previewTemplate, "## Acceptance", "## Tasks"), "A title", "missing ## Acceptance", 1},
		{"injection", previewTemplate, "title\n---\nrepo: injected", "control characters", 1},
		{"name", strings.ReplaceAll(previewTemplate, "name: {{name}}", "name: wrong"), "A title", "name must equal {{name}}", 1},
		{"control", previewTemplate, "carriage\rreturn", "control characters", 1},
		{"tab", previewTemplate, "tab\there", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, state, _ := previewFixture(t, tc.template)
			var out bytes.Buffer
			err := runWatchTest(previewArgs(config, tc.title), &out)
			if exitCode(err) != tc.code || (tc.message != "" && (err == nil || !strings.Contains(err.Error(), tc.message))) {
				t.Fatalf("code=%d err=%v", exitCode(err), err)
			}
			if tc.code == 0 && !strings.Contains(out.String(), "TEST-1 https://example.invalid/TEST-1 "+tc.title+" Ready") {
				t.Fatalf("output: %s", out.String())
			}
			if tc.code != 0 && out.Len() != 0 {
				t.Fatal("printed invalid order")
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Fatalf("state touched: %v", err)
			}
		})
	}
}
func TestRenderWhileLocked(t *testing.T) {
	config, state, _ := previewFixture(t, previewTemplate)
	if err := os.Mkdir(state, 0700); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockState(filepath.Join(state, "RUNNING"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if err := runWatchTest(previewArgs(config, "Title"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(state)
	if err != nil || len(entries) != 1 || entries[0].Name() != "RUNNING" {
		t.Fatalf("state changed: %v %v", entries, err)
	}
}
func TestRenderOnlineOneRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "secret" {
			t.Error("missing auth")
		}
		var body bytes.Buffer
		body.ReadFrom(r.Body)
		if !strings.Contains(body.String(), "RenderIssue") || !strings.Contains(body.String(), "TEST-1") || strings.Contains(body.String(), "mutation") {
			t.Errorf("query: %s", body.String())
		}
		fmt.Fprint(w, `{"data":{"issue":{"identifier":"TEST-1","url":"https://example.invalid/TEST-1","title":"Online title","state":{"name":"Ready"}}}}`)
	}))
	defer server.Close()
	config, state, _ := previewFixture(t, previewTemplate)
	c, err := loadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	p := previewClient(c, "secret")
	p.endpoint = server.URL
	i, err := p.renderIssue(context.Background(), "TEST-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = renderOrder(i, c.Status[0], previewName(i, c.Status[0])); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || p.day().Requests != 1 {
		t.Fatalf("calls=%d accounted=%d", calls, p.day().Requests)
	}
	if _, err = os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state touched: %v", err)
	}
}
func TestCheckEveryTemplate(t *testing.T) {
	config, state, tpl := previewFixture(t, strings.ReplaceAll(previewTemplate, "## Acceptance", "## Tasks"))
	extra := fmt.Sprintf("\n[[status]]\nname = 'Review'\naction = 'review'\nmode = 'dry-run'\nconcurrency = 'read'\ntemplate = %q\n[[status]]\nname = 'Off'\naction = 'off'\nmode = 'off'\nconcurrency = 'read'\ntemplate = 'missing'\n[[status]]\nname = 'No template'\naction = 'none'\nconcurrency = 'read'\n", tpl)
	f, err := os.OpenFile(config, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(extra)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	err = runWatchTest([]string{"linear", "check", "--config", config}, &bytes.Buffer{})
	if exitCode(err) != 1 || !strings.Contains(err.Error(), `rule "Ready"`) || !strings.Contains(err.Error(), `rule "Review"`) || strings.Contains(err.Error(), `rule "Off"`) {
		t.Fatalf("%v", err)
	}
	if err = os.WriteFile(tpl, []byte(previewTemplate), 0600); err != nil {
		t.Fatal(err)
	}
	if err = runWatchTest([]string{"linear", "check", "--config", config}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state touched: %v", err)
	}
}
func TestRenderUsage(t *testing.T) {
	config, _, _ := previewFixture(t, previewTemplate)
	for _, args := range [][]string{
		{"linear", "render", "--config", config},
		{"linear", "render", "--config", config, "--status", "Ready", "--ticket", "TEST-1", "--title", "mixed"},
		{"linear", "render", "--config", config, "--status", "Unknown", "--ticket", "TEST-1"},
		{"linear", "render", "--config", config, "--status", "Ready", "--ticket", "TEST-1"},
	} {
		t.Setenv("PREVIEW_TEST_KEY", "")
		if err := runWatchTest(args, &bytes.Buffer{}); exitCode(err) != 2 {
			t.Fatalf("%v: %v", args, err)
		}
	}
}

type previewTransport func(*http.Request) (*http.Response, error)

func (f previewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRenderOnlineCLI(t *testing.T) {
	config, state, _ := previewFixture(t, previewTemplate)
	t.Setenv("PREVIEW_TEST_KEY", "private-key")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"data":{"issue":{"identifier":"TEST-1","url":"https://example.invalid/TEST-1","title":"Online title","state":{"name":"Actual status"}}}}`)
	}))
	defer server.Close()
	original := http.DefaultTransport
	http.DefaultTransport = previewTransport(func(r *http.Request) (*http.Response, error) {
		target := *r.URL
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(server.URL, "http://")
		if target.String() != endpoint {
			t.Errorf("unexpected endpoint %s", target.String())
		}
		return original.RoundTrip(r)
	})
	defer func() { http.DefaultTransport = original }()
	args := []string{"linear", "render", "--config", config, "--status", "Ready", "--ticket", "TEST-1"}
	var out, diagnostics bytes.Buffer
	if err := runTemplate(args[1:], &out, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diagnostics.String(), `selected rule "Ready" differs from ticket status "Actual status"`) {
		t.Fatalf("warning: %s", diagnostics.String())
	}
	if calls != 1 || !strings.Contains(out.String(), "Online title Actual status") || strings.Contains(out.String(), "private-key") {
		t.Fatalf("calls=%d output=%s", calls, out.String())
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state touched: %v", err)
	}
	http.DefaultTransport = previewTransport(func(r *http.Request) (*http.Response, error) { return nil, fmt.Errorf("private-key") })
	err := runWatchTest(args, &bytes.Buffer{})
	if exitCode(err) != 2 || strings.Contains(err.Error(), "private-key") {
		t.Fatalf("network error: %v", err)
	}
}

func TestCheckNoTemplates(t *testing.T) {
	config, state, _ := previewFixture(t, previewTemplate)
	b, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "mode = 'live'", "mode = 'off'")
	if err := os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runWatchTest([]string{"linear", "check", "--config", config}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "No templated rules to check\n" {
		t.Fatalf("output: %s", out.String())
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatalf("state touched: %v", err)
	}
}

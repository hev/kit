package hookapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/hev/kit/internal/hooks"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}

func service(t *testing.T, outbound transport) *Service {
	t.Helper()
	// Workers are driven explicitly so tests can verify each queue transition.
	ctx := context.Background()
	s, err := openTestService(ctx, filepath.Join(t.TempDir(), "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	s.client.Transport = outbound
	return s
}

func openTestService(ctx context.Context, path string) (*Service, error) {
	return open(ctx, path, []byte(strings.Repeat("K", 32)))
}

func compiler(t *testing.T) *jsonschema.Compiler {
	t.Helper()
	raw, err := os.ReadFile("testdata/kit-v1-hooks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	// OpenAPI response objects can themselves be references. Resolve that
	// layer here; JSON Schema resolves references inside the selected schema.
	paths := document["paths"].(map[string]any)
	responses := document["components"].(map[string]any)["responses"].(map[string]any)
	for _, item := range paths {
		for _, method := range item.(map[string]any) {
			op, ok := method.(map[string]any)
			if !ok {
				continue
			}
			for code, node := range op["responses"].(map[string]any) {
				if ref, ok := node.(map[string]any)["$ref"].(string); ok {
					op["responses"].(map[string]any)[code] = responses[strings.TrimPrefix(ref, "#/components/responses/")]
				}
			}
		}
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("https://contract.invalid/kit", document); err != nil {
		t.Fatal(err)
	}
	return c
}

func validate(t *testing.T, c *jsonschema.Compiler, pointer string, raw []byte) {
	t.Helper()
	schema, err := c.Compile("https://contract.invalid/kit#/" + pointer)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(v); err != nil {
		t.Fatal(err)
	}
}

func TestHooksHTTPContract(t *testing.T) {
	const chosenSecret = "chosen-credential-that-must-not-leak"
	const content = "hidden-prompt-that-must-not-be-stored"
	eventID := "01jd2x8c7k4m5n6p7q8r9s0t1v"
	var attempts atomic.Int32
	c := compiler(t)
	s := service(t, func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Hev-Timestamp") == "" || !strings.HasPrefix(r.Header.Get("X-Hev-Delivery-Id"), "dlv_") ||
			r.Header.Get("X-Hev-Event") != "prompt-submit" || !strings.HasPrefix(r.Header.Get("X-Hev-Hook-Id"), "hook_") {
			t.Error("missing delivery headers")
		}
		if r.Header.Get("X-Hev-Test") == "" {
			attempts.Add(1)
			validate(t, c, "webhooks/blockingHookEvent/post/requestBody/content/application~1json/schema", body)
			if strings.Contains(string(body), content) {
				t.Error("included prompt without opt in")
			}
			if !strings.Contains(r.Header.Get("X-Hev-Signature-256"), sign(chosenSecret, r.Header.Get("X-Hev-Timestamp"), body)) {
				t.Error("rotation signature missing")
			}
		}
		return response(200, `{"decision":"halt","add_context":"policy context"}`), nil
	})
	owner := Identity{Account: "account", Author: "author", Host: "node"}
	call := func(id Identity, method, path, contractPath, body, status string) string {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		s.Serve(w, r, id)
		if !strings.HasPrefix(w.Result().Status, status+" ") {
			t.Fatalf("%s %s: %s %s", method, path, w.Result().Status, w.Body.String())
		}
		if status != "204" {
			pointer := "paths/" + strings.ReplaceAll(contractPath, "/", "~1") + "/" + strings.ToLower(method) + "/responses/" + status + "/content/application~1json/schema"
			validate(t, c, pointer, w.Body.Bytes())
		}
		return w.Body.String()
	}
	call(Identity{}, "GET", root, root, "", "401")
	call(owner, "GET", root+"?limit=0", root, "", "400")
	call(owner, "POST", root, root, `{"url":"https://127.0.0.1","events":["*"]}`, "422")
	call(owner, "POST", root, root, `{"url":"https://8.8.8.8"}`, "400")
	created := call(owner, "POST", root, root, `{"url":"https://8.8.8.8","events":["prompt-submit"],"blocking":true,"secret":"`+chosenSecret+`"}`, "201")
	var obj struct{ ID, Secret string }
	if json.Unmarshal([]byte(created), &obj) != nil || obj.Secret != chosenSecret {
		t.Fatal("create must return the supplied secret")
	}
	path := root + "/" + obj.ID
	template := root + "/{id}"
	call(Identity{Account: "other"}, "GET", path, template, "", "404")
	rotated := call(owner, "PATCH", path, template, `{"rotate_secret":true}`, "200")
	var next struct{ Secret string }
	if json.Unmarshal([]byte(rotated), &next) != nil || next.Secret == "" || next.Secret == chosenSecret {
		t.Fatal("expected fresh signing secret")
	}
	event := `{"schema":"hev.hook.event/v1","event":"prompt-submit","id":"` + eventID + `","time":"` + time.Now().UTC().Format(time.RFC3339Nano) + `","harness":"codex","prompt":"` + content + `","prompt_bytes":39,"cwd":"/private/path"}`
	for range 2 {
		result := call(owner, "POST", root+"/events", root+"/events", event, "200")
		if !strings.Contains(result, `"decision":"halt"`) {
			t.Fatal("lost gate answer")
		}
	}
	if attempts.Load() != 1 {
		t.Fatal("duplicate event was delivered again")
	}
	logs := call(owner, "GET", root+"/events?host=node&decision=halt", root+"/events", "", "200")
	test := call(owner, "POST", path+"/test", template+"/test", "", "200")
	if !strings.Contains(test, `"test":true`) {
		t.Fatal("test delivery missing marker")
	}
	reads := []string{
		logs, test,
		call(owner, "GET", root+"?event=prompt-submit", root, "", "200"),
		call(owner, "GET", path, template, "", "200"),
		call(owner, "GET", path+"/deliveries?limit=1", template+"/deliveries", "", "200"),
	}
	for _, v := range reads {
		for _, needle := range []string{chosenSecret, next.Secret, content, "/private/path", "policy context"} {
			if strings.Contains(v, needle) {
				t.Fatal("sensitive data appeared on a read")
			}
		}
	}
	raw, err := os.ReadFile(s.path + ".sqlite-wal")
	if err != nil || strings.Contains(string(raw), chosenSecret) || strings.Contains(string(raw), next.Secret) {
		t.Fatal("state must be encrypted")
	}
	loaded, err := openTestService(context.Background(), s.path)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.db.Close()
	if loaded.accounts[owner.Account].Hooks[obj.ID].Secret != next.Secret {
		t.Fatal("secret did not survive restart")
	}
	call(owner, "DELETE", path, template, "", "204")
	call(owner, "GET", path+"/deliveries", template+"/deliveries", "", "404")
}

func TestSignatureVectorAndReplayTimestamp(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	expected := "sha256=654f06c856baf080af3fa272934823257a542d35cf1f88099338f850a60601a4"
	if sign("secret", "1700000000", body) != expected || sign("secret", "1700000001", body) == expected {
		t.Fatal("signature must cover timestamp dot raw body")
	}
}

func TestTargetsAndRedirects(t *testing.T) {
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "::ffff:192.168.0.1", "fe80::1", "100.64.0.1"} {
		if publicAddress(netip.MustParseAddr(ip)) {
			t.Fatalf("accepted %s", ip)
		}
	}
	if !publicAddress(netip.MustParseAddr("8.8.8.8")) || !publicAddress(netip.MustParseAddr("2606:4700::1111")) {
		t.Fatal("public addresses rejected")
	}
	for _, url := range []string{"http://8.8.8.8", "https://[::1]", "https://me@8.8.8.8"} {
		if validateURL(context.Background(), url) == nil {
			t.Fatalf("accepted %s", url)
		}
	}
	var calls int
	s := service(t, func(r *http.Request) (*http.Response, error) {
		calls++
		out := response(307, "")
		out.Header.Set("Location", "https://redirect.invalid")
		return out, nil
	})
	a, _ := s.send(context.Background(), hook{URL: "https://example.invalid", Secret: "test"}, delivery{}, []byte("{}"))
	if calls != 1 || a.Code == nil || *a.Code != 307 {
		t.Fatal("signature was sent to a redirect")
	}
}

func TestRetryAndBlockingFailureRules(t *testing.T) {
	start := time.Now()
	observe := delivery{}
	wantDelays := []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour, 15 * time.Hour}
	for i := 1; i <= 7; i++ {
		settle(&observe, attempt{N: i, Error: "timeout"}, hooks.Response{}, start)
		if i <= len(wantDelays) && (observe.Status != "retrying" || !observe.Next.Equal(start.Add(wantDelays[i-1]))) {
			t.Fatal("observe retry did not follow contract schedule")
		}
	}
	if observe.Status != "failed" || observe.Next != nil {
		t.Fatal("observe retry did not stop")
	}
	var calls int
	for _, code := range []int{200, 410, 204} {
		s := service(t, func(r *http.Request) (*http.Response, error) { calls++; return response(code, "invalid JSON"), nil })
		d := delivery{Blocking: true, Event: "pre-tool"}
		a, answer := s.send(context.Background(), hook{URL: "https://example.invalid", Timeout: 100}, d, nil)
		settle(&d, a, answer, start)
		if d.Decision != "allow" || d.Next != nil || d.FailedOpen != (code != 204) {
			t.Fatalf("incorrect blocking result for %d", code)
		}
	}
	if calls != 3 {
		t.Fatal("blocking delivery retried")
	}
}

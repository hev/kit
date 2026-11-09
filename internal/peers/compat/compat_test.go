package compat

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Goldens below were produced by CPython 3.14 json.dumps / repr / shlex.quote.

func TestFloatReprMatchesPython(t *testing.T) {
	for in, want := range map[float64]string{
		1794150352.3297715: "1794150352.3297715", 1.0: "1.0", 0.0001: "0.0001", 1e-05: "1e-05", 1e16: "1e+16",
		123456789012345678.0: "1.2345678901234568e+17", -2.5: "-2.5", 1e22: "1e+22", 0.1: "0.1", 100.0: "100.0", 1731000000.0: "1731000000.0",
	} {
		if got := FloatRepr(in); got != want {
			t.Errorf("FloatRepr(%v) = %s, want %s", in, got, want)
		}
	}
}

func TestDumpsMatchesPython(t *testing.T) {
	o := NewObj("b", 1, "a", []any{1, 2.5, nil, true, "é☃😀\n\"\\\x01\x7f"}, "c", NewObj())
	if got, want := Dumps(o), `{"b": 1, "a": [1, 2.5, null, true, "\u00e9\u2603\ud83d\ude00\n\"\\\u0001\u007f"], "c": {}}`; got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	indented := Dumps(NewObj("b", 1, "a", []any{1, NewObj()}, "c", NewObj("x", []any{})), 1)
	if want := "{\n \"b\": 1,\n \"a\": [\n  1,\n  {}\n ],\n \"c\": {\n  \"x\": []\n }\n}"; indented != want {
		t.Fatalf("\n got %s\nwant %s", indented, want)
	}
	if got := DumpsUnicode("é😀"); got != `"é😀"` {
		t.Fatal(got)
	}
}

func TestLoadsKeepsOrderAndNumberKinds(t *testing.T) {
	in := `{"z": 1, "a": 2.0, "m": {"y": [1, 1.5e3]}, "big": 123456789012345678901234567890}`
	v, err := Loads([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := Dumps(v), `{"z": 1, "a": 2.0, "m": {"y": [1, 1500.0]}, "big": 123456789012345678901234567890}`; got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	o := v.(*Obj)
	merged := o.Merge(NewObj("a", "x", "new", true))
	if strings.Join(merged.Keys(), ",") != "z,a,m,big,new" || merged.Str("a") != "x" {
		t.Fatal(Dumps(merged))
	}
	if !Equal(1, 1.0) || Equal(true, 1) || !Equal(NewObj("a", 1), NewObj("a", 1.0)) {
		t.Fatal("Python equality")
	}
}

// Python keeps lone surrogates (for example a surrogateescape path byte) and
// writes them back as the same escape; so must a Go read and rewrite.
var surrogateCases = []string{
	`"\udcff"`, `"\ud800"`, `"\udfff"`, `"a\ud83d\ude00b"`, `"\ud83dx\ude00"`, `"\ude00\ud83d"`,
	`"\ud83d\ud83d\ude00"`, `"\ud83d\u0041"`, `"\ud83d\u"`, `{"cwd": "/tmp/\udcff\udcfe", "message": ["\udc80", "\u00e9\udced\udca0\udc80"]}`,
	`"\udced\udcb3\udcbf"`, `"\ud83d"`,
}

func TestLoadsKeepsLoneSurrogates(t *testing.T) {
	for _, in := range surrogateCases {
		v, err := Loads([]byte(in))
		if strings.HasSuffix(in, `\u"`) {
			if err == nil {
				t.Errorf("%s: truncated escape accepted", in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		want := strings.ReplaceAll(in, `\u0041`, "A")
		if got := Dumps(v); got != want {
			t.Errorf("Dumps(Loads(%s)) = %s", in, got)
		}
		// An ensure_ascii=False layer keeps them for the outer dumps to escape
		// once, so only the inner quotes change.
		if got := Dumps(DumpsUnicode(v)); got != `"`+strings.ReplaceAll(want, `"`, `\"`)+`"` {
			t.Errorf("Dumps(DumpsUnicode(Loads(%s))) = %s", in, got)
		}
	}
	if got := Dumps("/tmp/\xff\xfex"); got != `"/tmp/\udcff\udcfex"` {
		t.Errorf("non-UTF-8 OS bytes: %s", got)
	}
	if got := Dumps(DumpsUnicode("\xff")); got != `"\"\udcff\""` {
		t.Errorf("non-UTF-8 OS bytes through ensure_ascii=False: %s", got)
	}
}

// TestLoadsMatchesPythonJSON compares json.dumps(json.loads(x)) with Python
// when python3 is available.
func TestLoadsMatchesPythonJSON(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		if os.Getenv("PEERS_INTEROP_REQUIRED") == "1" {
			t.Fatal("python3 is required")
		}
		t.Skip("python3 not installed")
	}
	inputs := append(append([]string{}, surrogateCases[:8]...), surrogateCases[9:]...)
	inputs = append(inputs, `{"a": 1, "a": 2, "b": NaN, "c": -Infinity, "d": 1E400, "e": -0.0, "f": 1e5, "g": "\/"}`,
		`[ 1 , {"x" : [ ] } ]`, "\t{\"k\": \"\u00e9\u2603\"}\n")
	script := `import json,sys
for line in sys.stdin.read().split("\x00"):
    print(json.dumps(json.loads(line)))
    print(json.dumps(json.dumps(json.loads(line), ensure_ascii=False)))`
	cmd := exec.Command(python, "-c", script)
	cmd.Stdin = strings.NewReader(strings.Join(inputs, "\x00"))
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	for i, in := range inputs {
		v, err := Loads([]byte(in))
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got := Dumps(v); got != want[2*i] {
			t.Errorf("%s: Go %s, Python %s", in, got, want[2*i])
		}
		if got := Dumps(DumpsUnicode(v)); got != want[2*i+1] {
			t.Errorf("%s (ensure_ascii=False): Go %s, Python %s", in, got, want[2*i+1])
		}
	}
}

func TestTmuxLineAcceptsEscapedSeparator(t *testing.T) {
	for in, want := range map[string]string{
		`%0\037alpha\0370`:   "%0\x1falpha\x1f0",
		"%0\x1fa\\037b\x1f0": "%0\x1fa\\037b\x1f0", // a real separator wins; the text stays
		"plain":              "plain",
	} {
		if got := tmuxLine(in); got != want {
			t.Errorf("tmuxLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShlexQuoteMatchesPython(t *testing.T) {
	for in, want := range map[string]string{"": "''", "abc": "abc", "a b": "'a b'", "it's": `'it'"'"'s'`, "/x/y-z.py": "/x/y-z.py",
		"é": "'é'", "a=b,c:d@e%f+g": "a=b,c:d@e%f+g"} {
		if got := shlexQuote(in); got != want {
			t.Errorf("shlexQuote(%q) = %s, want %s", in, got, want)
		}
		if words, ok := shlexSplit(shlexQuote(in) + " tail"); !ok || words[0] != in || words[1] != "tail" {
			t.Errorf("shlexSplit round trip %q: %q", in, words)
		}
	}
}

func TestSplitlinesMatchesPython(t *testing.T) {
	if got := strings.Join(splitlines("a\rb\r\nc\x0bd\x1ce f\n"), "|"); got != "a|b|c|d|e|f" {
		t.Fatal(got)
	}
}

func TestClassify(t *testing.T) {
	for cmd, want := range map[string]string{
		"claude 600": "claude", "/usr/bin/node /x/@anthropic-ai/claude-code/cli.js": "claude", "codex exec hi": "codex",
		"node /x/bin/codex.js": "codex", "pi": "pi", "node /lib/@earendil-works/pi-coding-agent/dist/cli.js": "pi",
		"node /x/gemini": "gemini", "opencode": "opencode", "bash": "", "pi --foo": "",
	} {
		if got := classify(cmd); got != want {
			t.Errorf("classify(%q) = %q, want %q", cmd, got, want)
		}
	}
	if !headless("claude", "claude -p hi") || headless("claude", "claude") || !headless("codex", "codex exec x") || headless("codex", "codex resume x exec") {
		t.Fatal("headless")
	}
	if got := promptOf(`claude -p fix the\012bug   now`); got != "fix the bug now" {
		t.Fatal(got)
	}
}

func TestFindAndAddress(t *testing.T) {
	rows := []*Row{
		{ID: "%1", Session: "one", Window: "0", Pane: "0", Agent: "claude"},
		{ID: "%2", Session: "two", Window: "0", Pane: "0", Agent: "claude"},
		{ID: "%3", Session: "two", Window: "1", Pane: "0", Agent: "codex"},
		{ID: "%4", Session: "shell", Window: "0", Pane: "0"},
	}
	p := &P{}
	if r, err := find("one", rows); err != nil || r.ID != "%1" || p.address(r, rows) != "one" {
		t.Fatal(r, err)
	}
	if _, err := find("two", rows); err == nil || err.Error() != "session two has 2 agent panes; pick one: %2 (claude), %3 (codex)" {
		t.Fatal(err)
	}
	if r, err := find("two:1.0", rows); err != nil || r.ID != "%3" || p.address(r, rows) != "%3" {
		t.Fatal(r, err)
	}
	if _, err := find("it's", rows); err == nil || err.Error() != `no pane matches "it's" (peers ls to see who's here)` {
		t.Fatal(err)
	}
	if h, rest := splitHost("mini/%3"); h != "mini" || rest != "%3" {
		t.Fatal(h, rest)
	}
}

func TestRelayFingerprintIgnoresTransportFields(t *testing.T) {
	a := NewObj("id", "x", "from_host", "mini", "message", "hi", "thread", NewObj("id", "abc", "title", "t"), "ts", 1.0)
	b := NewObj("message", "hi", "from_host", "mini", "id", "y", "thread", "abc", "via", "laptop")
	if relayFingerprint(a) != relayFingerprint(b) {
		t.Fatal("fingerprint covers only content, route and thread id")
	}
	// sha256 of json.dumps({...}, sort_keys=True) for this payload, from CPython.
	if got := relayFingerprint(NewObj("message", "hi")); got != "8bcaa440202dd5dd28e86b98df5925da683f56f43ac860113b1c306abc25a7db" {
		t.Fatal(got)
	}
}

// Dispatch notifications write through the native engine, which honours
// HEV_PEERS_STATE; the compat CLI must read the same tree.
func TestStateDirFollowsNativeOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", os.Getenv("PATH"))
	state := t.TempDir()
	t.Setenv("HEV_PEERS_STATE", state)
	if p := newP(nil, io.Discard, io.Discard); p.stateDir != state || p.inboxDir != filepath.Join(state, "inbox") {
		t.Fatalf("stateDir %q inboxDir %q, want %q", p.stateDir, p.inboxDir, state)
	}
	t.Setenv("HEV_PEERS_STATE", "")
	if p := newP(nil, io.Discard, io.Discard); p.stateDir != filepath.Join(os.Getenv("HOME"), ".local/state/peers-v2") {
		t.Fatalf("default stateDir %q", p.stateDir)
	}
}

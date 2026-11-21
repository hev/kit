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

// Lines below are real tmux 3.4 (Ubuntu 3.4-1build1) list-panes output for a
// #{q:...} format, LC_ALL=C, -u: pane title `literal\037title`, cwd
// `/tmp/w\037d $x\y`.
const tmux34Plain = `%0\037alpha\0370\0370\0371313881\037/tmp/w\037d \$x\y\037literal\037title\0370\0371794271619\037\0371313880:$0:1794271619`
const tmux34Quoted = `\%0\037alpha\0370\0370\0371313881\037/tmp/w\\037d\ \\$x\\y\037literal\\037title\0370\0371794271619\037\0371313880:\$0:1794271619`

func TestDecodeTmuxQSplitsTmux34Unambiguously(t *testing.T) {
	want := []string{"%0", "alpha", "0", "0", "1313881", `/tmp/w\037d $x\y`, `literal\037title`, "0", "1794271619", "", "1313880:$0:1794271619"}
	got := decodeTmuxQ(tmux34Quoted)
	if strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
		t.Fatalf("decodeTmuxQ:\n got %q\nwant %q", got, want)
	}
	for in, want := range map[string][]string{
		`a\\\\$b`:     {`a\$b`},      // field \$b: q \\\$b, then the output's \$ escape
		`a\\\001b`:    {"a\\\x01b"},  // field \ followed by a control byte
		`\$0\037x\ y`: {"$0", "x y"}, // "$0" is not escaped by the output encoding
		`\\\\\037\\`:  {`\\`, `\`},
		"":            {""},
	} {
		if got := decodeTmuxQ(in); strings.Join(got, "|") != strings.Join(want, "|") || len(got) != len(want) {
			t.Errorf("decodeTmuxQ(%q) = %q, want %q", in, got, want)
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
		"agy --settings /tmp/settings.json": "agy", "/path/to/agy": "agy",
		"fzf --prompt harness: agy --settings": "", "fzf /path/to/agy": "",
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

func TestAgyHeadless(t *testing.T) {
	for command, want := range map[string]bool{
		"agy --settings settings.json": false,
		"agy --print=prompt":           true,
		"agy --print prompt":           true,
		"agy -p prompt":                true,
		"agy --prompt=prompt":          true,
		"agy --prompt-interactive hi":  false,
		"agy -- --print":               false,
	} {
		if got := headless("agy", command); got != want {
			t.Errorf("headless(%q) = %v, want %v", command, got, want)
		}
	}
}

func TestAgyScreenState(t *testing.T) {
	for _, tt := range agyScreens {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.screen, "\n")
			if got := screenState(tt.agent, lines, lines); got != tt.want {
				t.Errorf("state = %q, want %q", got, tt.want)
			}
		})
	}
}

const (
	agyRule         = "────────────────────────"
	agyIdle         = "? for shortcuts          Gemini 4 Neon · medium"
	agyCancelFooter = "esc to cancel · 12s          Gemini 4 Neon · medium"
	agyEditor       = agyRule + "\n>  \n" + agyRule + "\n"
)

// agyScreens: agy's running footer reads "esc to cancel" and is working; a
// real dialog or the feedback survey is a prompt; Claude and Codex keep
// "Esc to cancel" as a dialog. The native engine keeps a copy of this table.
var agyScreens = []struct{ name, agent, screen, want string }{
	{"empty editor", "agy", agyEditor + agyIdle, "idle"},
	{"typed editor", "agy", agyRule + "\n> unsent text\n" + agyRule + "\n" + agyIdle, "draft"},
	{"numbered draft", "agy", agyRule + "\n> 1. unsent text\n" + agyRule + "\n" + agyIdle, "draft"},
	{"multiline draft", "agy", agyRule + "\n>  \nsecond line\n" + agyRule + "\n" + agyIdle, "draft"},
	{"blockquote above editor", "agy", "> some quote\n" + agyEditor + agyIdle, "idle"},
	{"blockquote alone", "agy", "> some quote", "?"},
	{"esc to interrupt", "agy", "Thinking… (esc to interrupt)\n" + agyEditor + agyIdle, "working"},
	{"finished thought is not a turn", "agy", "○ Thought for 3s\n" + agyEditor + agyIdle, "idle"},
	{"cancel footer under editor", "agy", agyEditor + agyCancelFooter, "working"},
	{"cancel footer with spinner", "agy", "⣯ Running command...\n" + agyEditor + agyCancelFooter, "working"},
	{"cancel footer after thought", "agy", "○ Thought for 3s\n⣽ Reading files...\n" + agyEditor + agyCancelFooter, "working"},
	{"cancel footer, spinner, no editor", "agy", "⣯ Running command...\n" + agyCancelFooter, "working"},
	{"spinner alone", "agy", "⣾ Generating...", "working"},
	{"draft while running", "agy", "⣯ Running command...\n" + agyRule + "\n> next idea\n" + agyRule + "\n" + agyCancelFooter, "draft"},
	{"typed cancel words are a draft", "agy", agyRule + "\n> esc to cancel\n" + agyRule + "\n" + agyIdle, "draft"},
	{"cancel footer alone is a dialog", "agy", "Allow this command?\n  1. Yes\n  2. No\nEsc to cancel", "prompt"},
	{"dialog", "agy", "Do you want to proceed?\n> 1. Yes\n  2. No", "prompt"},
	{"dialog during a turn", "agy", "⣯ Running command...\nDo you want to proceed?\n> 1. Yes\n  2. No\nEsc to cancel", "prompt"},
	{"feedback survey", "agy", "○ Thought for 3s\nHow's the CLI experience so far? Help us improve:\n[1] Good  [2] Fine  [3] Bad  [0] Skip", "prompt"},
	{"survey choices alone", "agy", "[1] Good  [2] Fine  [3] Bad  [0] Skip", "prompt"},
	{"survey above editor", "agy", "How's the CLI experience so far? Help us improve:\n" + agyEditor + agyIdle, "prompt"},
	{"claude dialog", "claude", "Do you want to make this edit?\n❯ 1. Yes\nEsc to cancel", "prompt"},
	{"claude cancel footer", "claude", "Esc to cancel\n❯ ", "prompt"},
	{"codex cancel footer", "codex", "Esc to cancel\n› ", "prompt"},
	{"claude esc to interrupt", "claude", "✻ Thinking… (12s · esc to interrupt)\n❯ ", "working"},
	{"codex esc to interrupt", "codex", "• Working (3s • esc to interrupt)\n› ", "working"},
	{"claude idle", "claude", "❯ ", "idle"},
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
	// A fresh HOME keeps peers under the unified ~/.hev/peers ...
	if p := newP(nil, io.Discard, io.Discard); p.stateDir != filepath.Join(os.Getenv("HOME"), ".hev", "peers") {
		t.Fatalf("default stateDir %q", p.stateDir)
	}
	// ... and a host that already has the legacy tree keeps reading it.
	legacy := filepath.Join(os.Getenv("HOME"), ".local", "state", "peers-v2")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if p := newP(nil, io.Discard, io.Discard); p.stateDir != legacy {
		t.Fatalf("legacy stateDir %q", p.stateDir)
	}
}

// Strings below are WTF-8: "\xed\xb3\xbf" is the lone surrogate U+DCFF, as a
// Python surrogateescape byte 0xFF decodes. Expectations are Python's.
func TestStringTransformsKeepSurrogates(t *testing.T) {
	const s = "a\xed\xb3\xbfb\xed\xa0\x80c😀d" // 'a\udcffb\ud800c😀d': 7 code points
	if n := pyLen(s); n != 7 {
		t.Fatalf("pyLen = %d", n)
	}
	for n, want := range map[int]string{0: "", 1: "a", 2: "a\xed\xb3\xbf", 4: "a\xed\xb3\xbfb\xed\xa0\x80", 6: "a\xed\xb3\xbfb\xed\xa0\x80c😀", 7: s, 300: s} {
		if got := truncRunes(s, n); got != want {
			t.Errorf("truncRunes(s, %d) = %q, want %q", n, got, want)
		}
	}
	if got, want := splitlines("x\xed\xb3\xbf\vy\x1cz\r\n\xed\xa0\x80"), []string{"x\xed\xb3\xbf", "y", "z", "\xed\xa0\x80"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("splitlines = %q, want %q", got, want)
	}
	// re.sub(r"\s+", " ", s): \v, \x1c-\x1f, \x85 and U+3000 are whitespace.
	if got, want := spaceRE.ReplaceAllString("a\v\x1f\u0085　b\xed\xb3\xbf\t c", " "), "a b\xed\xb3\xbf c"; got != want {
		t.Errorf("spaceRE = %q, want %q", got, want)
	}
	if got := pyStrip("\x1c\x1f x\xed\xb3\xbf \x1e"); got != "x\xed\xb3\xbf" {
		t.Errorf("pyStrip = %q", got)
	}
	if got := pad("\xed\xb3\xbf", 3); got != "\xed\xb3\xbf  " {
		t.Errorf("pad = %q", got)
	}
}

func TestPyStreamEncodesLikePython(t *testing.T) {
	// print('a\udcffb\ud800c') under C.UTF-8: stdout surrogateescape (U+D800
	// raises there and passes through here), stderr backslashreplace.
	in := "a\xed\xb3\xbfb\xed\xa0\x80cé\n"
	var out, errb strings.Builder
	io.WriteString(pyStream{w: &out}, in)
	io.WriteString(pyStream{w: &errb, stderr: true}, in)
	if got, want := out.String(), "a\xffb\xed\xa0\x80cé\n"; got != want {
		t.Errorf("stdout %q, want %q", got, want)
	}
	if got, want := errb.String(), `a\udcffb\ud800cé`+"\n"; got != want {
		t.Errorf("stderr %q, want %q", got, want)
	}
}

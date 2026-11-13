//go:build !windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hev/kit/internal/secrets/fakeazure"
	"github.com/hev/kit/internal/secrets/fakeop"
)

// setHostManager writes the host's kit config with a [secrets] table (body
// "" removes the file) for the in-process daemon and every hev the scripts
// run.
func (h *shellHost) setHostManager(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(h.home, ".hev", "config.toml")
	os.MkdirAll(filepath.Dir(p), 0o700)
	if body == "" {
		os.Remove(p)
	} else if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"HEV_CONFIG", "HEV_SECRETS_MANAGER", "HEV_SECRETS_VAULT", "HEV_SECRETS_FIELD"} {
		t.Setenv(k, "")
	}
	h.env = append(h.env, "HEV_CONFIG=", "HEV_SECRETS_MANAGER=", "HEV_SECRETS_VAULT=", "HEV_SECRETS_FIELD=")
	return p
}

const (
	onePasswordHost = "[secrets]\nmanager = \"1password\"\nvault = \"example\"\n"
	keyVaultHost    = "[secrets]\nmanager = \"azure-key-vault\"\nvault = \"" + fakeazure.Vault + "\"\n"
)

// The order's switch test: one .env file (the old value commented out, a
// secret:// reference under it) and one `hev secrets read secret://...` work
// on a 1Password host, then unchanged after the host is switched to Key
// Vault. Only the host's config changes; the daemon keeps running.
func TestSecretsNeutralReferenceSurvivesManagerSwitch(t *testing.T) {
	h := newShellHost(t, true)
	f := h.withAzure(t)
	h.setHostManager(t, onePasswordHost)
	h.serve(t)
	envFile := filepath.Join(h.root, ".env")
	body := "# STRIPE_API_KEY=sk_test_old_value_on_disk\nSTRIPE_API_KEY=secret://" + fakeazure.Secret + "\nPORT=8080\n"
	if err := os.WriteFile(envFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	run := `hev secrets run --env-file .env -- sh -c 'test "$STRIPE_API_KEY" = "$WANT" && test "$PORT" = 8080 && echo run-ok'`
	read := `x=$(hev secrets read --label STRIPE_API_KEY secret://` + fakeazure.Secret + `) && test "$x" = "$WANT" && echo read-ok`
	check := func(stage, want string) {
		t.Helper()
		env := append(append([]string(nil), h.env...), "WANT="+want)
		saved := h.env
		h.env = env
		defer func() { h.env = saved }()
		for _, s := range []string{run, read} {
			out, errs, err := h.sh(t, "sh", s)
			if err != nil || !strings.HasSuffix(strings.TrimSpace(out), "-ok") || strings.Contains(errs, "fallback") {
				t.Fatalf("%s: %v %q %q", stage, err, out, errs)
			}
		}
	}
	check("1Password host", fakeop.Value)
	if h.op.Count(t) != 1 || f.VaultGets() != 0 {
		t.Fatalf("1Password host: op %d, vault %d", h.op.Count(t), f.VaultGets())
	}
	if lines := h.op.CallLines(t); len(lines) != 1 || !strings.HasPrefix(lines[0], "inject") {
		t.Fatalf("op calls: %q", lines)
	}

	h.setHostManager(t, keyVaultHost)
	check("Key Vault host", fakeazure.Value)
	if h.op.Count(t) != 1 || f.VaultGets() != 1 {
		t.Fatalf("Key Vault host: op %d, vault %d (want no new op call and one vault read)", h.op.Count(t), f.VaultGets())
	}
	if b, _ := os.ReadFile(envFile); string(b) != body {
		t.Fatal("the env file changed")
	}
	argv, _ := os.ReadFile(h.argv)
	if strings.Contains(string(argv), fakeop.Value) || strings.Contains(string(argv), fakeazure.Value) {
		t.Fatal("a value reached hev's arguments")
	}
	h.noValueAnywhere(t)
	h.noKeyVaultSecretAnywhere(t)
}

// With no manager set, a secret:// read fails before any request and names
// the setting; explicit references on the same host still work.
func TestSecretsNeutralNoManagerCLI(t *testing.T) {
	h := newShellHost(t, true)
	path := h.setHostManager(t, "")
	h.serve(t)
	_, errs, err := h.sh(t, "sh", `hev secrets read secret://db-password`)
	if err == nil {
		t.Fatal("read succeeded with no manager")
	}
	for _, want := range []string{"secret:// needs this host's secret manager", path, "[secrets] manager", "HEV_SECRETS_MANAGER"} {
		if !strings.Contains(errs, want) {
			t.Errorf("stderr lacks %q: %q", want, errs)
		}
	}
	if _, errs, err = h.sh(t, "sh", `hev secrets read secret://DB_PASSWORD`); err == nil || !strings.Contains(errs, "lowercase letters, digits and hyphens") {
		t.Fatalf("malformed name: %v %q", err, errs)
	}
	if h.op.Count(t) != 0 {
		t.Fatal("a request was made")
	}
	if out, errs, err := h.sh(t, "sh", `hev secrets read op://example/db/password`); err != nil || strings.TrimSpace(out) != fakeop.Value {
		t.Fatalf("explicit reference: %v %q", err, errs)
	}
}

// Env files: comments and blank lines ignored, `export `, single and double
// quotes, an inline comment, a later duplicate line winning, --ref winning
// over the file, --ttl naming a file reference, plain values passing through
// and a file mixing secret://, op:// and azkv://, resolved in one request per
// store.
func TestSecretsEnvFile(t *testing.T) {
	h := newShellHost(t, true)
	f := h.withAzure(t)
	h.setHostManager(t, keyVaultHost)
	h.serve(t)
	file := filepath.Join(h.root, "app.env")
	os.WriteFile(file, []byte(strings.Join([]string{
		"# app settings",
		"",
		"   # indented comment",
		"export NEUTRAL=secret://" + fakeazure.Secret,
		"OP_KEY='op://example/db/password'",
		`KV_KEY="` + fakeazure.Ref() + `"   # trailing comment`,
		"PLAIN=hello world # inline comment",
		"HASH=a#b",
		`DOUBLE="line one\nsay \"hi\" \$HOME"`,
		"EMPTY=",
		"DUP=first",
		"DUP=second",
		"OVERRIDE=plain-in-file",
		"export\tTABBED=tab\r",
	}, "\n")), 0o600)

	out, errs, err := h.sh(t, "sh", `hev secrets env --env-file app.env --ref OVERRIDE=op://example/override/password --ttl KV_KEY=1h --format json`)
	if err != nil {
		t.Fatalf("env: %v %q", err, errs)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"NEUTRAL": fakeazure.Value, "OP_KEY": fakeop.Value, "KV_KEY": fakeazure.Value, "OVERRIDE": fakeop.Value,
		"PLAIN": "hello world", "HASH": "a#b", "DOUBLE": "line one\nsay \"hi\" $HOME", "EMPTY": "", "DUP": "second", "TABBED": "tab",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %d names, want %d", len(got), len(want))
	}
	if h.op.Count(t) != 1 || f.VaultGets() != 1 {
		t.Fatalf("want one request per store: op %d, vault %d", h.op.Count(t), f.VaultGets())
	}
}

// The quoting forms one at a time, and the sh format, which a calling shell
// evals: plain values come back exactly, in file order.
func TestSecretsEnvFileQuotingAndShellFormat(t *testing.T) {
	h := newShellHost(t, true)
	h.setHostManager(t, onePasswordHost)
	h.serve(t)
	os.WriteFile(filepath.Join(h.root, "q.env"), []byte(
		"export A=secret://db-password\n"+
			"B='single # quoted $HOME'\n"+
			"C=\"double\\tquoted \\\"x\\\"\"\n"+
			"D=bare value   # comment\n"+
			"B=later-b\n"), 0o600)
	out, errs, err := h.sh(t, "sh", `hev secrets env --env-file q.env`)
	if err != nil {
		t.Fatalf("env: %v %q", err, errs)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "export A=") || lines[1] != "export B='later-b'" || !strings.HasPrefix(lines[2], "export C=") || lines[3] != "export D='bare value'" {
		t.Fatalf("export lines: %q", lines)
	}
	script := `eval "$(hev secrets env --env-file q.env)" && test "$A" = "$WANT" && test "$B" = later-b && test "$C" = "$(printf 'double\tquoted "x"')" && test "$D" = "bare value" && echo ok`
	h.env = append(h.env, "WANT="+fakeop.Value)
	if out, errs, err := h.sh(t, "sh", script); err != nil || strings.TrimSpace(out) != "ok" {
		t.Fatalf("eval: %v %q %q", err, out, errs)
	}
	os.WriteFile(filepath.Join(h.root, "s.env"), []byte("S='single # quoted $HOME'\n"), 0o600)
	if out, _, err := h.sh(t, "sh", `hev secrets env --env-file s.env --format json`); err != nil || strings.TrimSpace(out) != `{"S":"single # quoted $HOME"}` {
		t.Fatalf("single quotes: %v %q", err, out)
	}
	// A file of plain values only makes no request.
	os.WriteFile(filepath.Join(h.root, "p.env"), []byte("X=1\n"), 0o600)
	if out, _, err := h.sh(t, "sh", `hev secrets env --env-file p.env`); err != nil || strings.TrimSpace(out) != "export X='1'" {
		t.Fatalf("plain only: %v %q", err, out)
	}
	if h.op.Count(t) != 1 {
		t.Fatalf("op ran %d times", h.op.Count(t))
	}
}

// Malformed files and flags fail without echoing a line's content.
func TestSecretsEnvFileErrors(t *testing.T) {
	h := newShellHost(t, true)
	h.setHostManager(t, onePasswordHost)
	cases := map[string]string{
		"bad name":         "1BAD=x-value-not-echoed\n",
		"no equals":        "JUST_A_NAME-value-not-echoed\n",
		"unterminated":     "A='x-value-not-echoed\n",
		"text after quote": "A=\"x-value-not-echoed\" tail\n",
		"bad reference":    "A=secret://Not_Portable\n",
	}
	for name, body := range cases {
		os.WriteFile(filepath.Join(h.root, "bad.env"), []byte(body), 0o600)
		_, errs, err := h.sh(t, "sh", `hev secrets env --env-file bad.env`)
		if err == nil || strings.Contains(errs, "value-not-echoed") {
			t.Errorf("%s: %v %q", name, err, errs)
		}
		if name != "bad reference" && !strings.Contains(errs, "line 1") {
			t.Errorf("%s: no line number: %q", name, errs)
		}
	}
	os.WriteFile(filepath.Join(h.root, "ok.env"), []byte("PLAIN=x\n"), 0o600)
	if _, errs, err := h.sh(t, "sh", `hev secrets env --env-file ok.env --ttl PLAIN=1h`); err == nil || !strings.Contains(errs, "TTL has no matching reference") {
		t.Errorf("TTL on a plain value: %v %q", err, errs)
	}
	if _, errs, err := h.sh(t, "sh", `hev secrets env --env-file missing.env`); err == nil || !strings.Contains(errs, "cannot open") {
		t.Errorf("missing file: %v %q", err, errs)
	}
	if _, errs, err := h.sh(t, "sh", `hev secrets env`); err == nil || !strings.Contains(errs, "--ref or --env-file") {
		t.Errorf("no input: %v %q", err, errs)
	}
	if h.op.Count(t) != 0 {
		t.Fatal("a request was made")
	}
}

// run --env-file: references and plain values reach the child's environment
// only; --ref wins over the file.
func TestSecretsRunEnvFile(t *testing.T) {
	h := newShellHost(t, true)
	h.setHostManager(t, onePasswordHost)
	h.serve(t)
	os.WriteFile(filepath.Join(h.root, ".env"), []byte("TOKEN=secret://api-token\nMODE=production\nOVER=file\n"), 0o600)
	h.env = append(h.env, "WANT="+fakeop.Value, "MODE=from-parent")
	out, errs, err := h.sh(t, "sh", `hev secrets run --env-file .env --ref OVER=op://example/over/password -- sh -c 'test "$TOKEN" = "$WANT" && test "$MODE" = production && test "$OVER" = "$WANT" && echo ok'`)
	if err != nil || strings.TrimSpace(out) != "ok" {
		t.Fatalf("run: %v %q %q", err, out, errs)
	}
	if h.op.Count(t) != 1 {
		t.Fatalf("op ran %d times", h.op.Count(t))
	}
	h.noValueAnywhere(t)
}

// status and status --json show the host's manager and where it is set, and
// what to set when it is not.
func TestSecretsStatusShowsManager(t *testing.T) {
	h := newShellHost(t, true)
	path := h.setHostManager(t, "")
	out, _, _ := h.sh(t, "sh", `hev secrets status --no-quota`)
	if !strings.Contains(out, "secret manager:  not set") || !strings.Contains(out, path) || !strings.Contains(out, "HEV_SECRETS_MANAGER") {
		t.Fatalf("unset status:\n%s", out)
	}
	h.setHostManager(t, keyVaultHost)
	h.serve(t)
	out, errs, err := h.sh(t, "sh", `hev secrets status --no-quota`)
	if err != nil || !strings.Contains(out, "secret manager:  Azure Key Vault (vault "+fakeazure.Vault+"; "+path+" [secrets])") {
		t.Fatalf("status: %v %q\n%s", err, errs, out)
	}
	t.Logf("hev secrets status (Key Vault manager):\n%s", out)
	out, errs, err = h.sh(t, "sh", `hev secrets status --no-quota --json`)
	if err != nil {
		t.Fatalf("status --json: %v %q", err, errs)
	}
	var s struct {
		Manager struct {
			Manager, Name, Vault, Source string
			Configured                   bool
		} `json:"manager"`
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatal(err)
	}
	if s.Manager.Manager != "azure-key-vault" || s.Manager.Name != "Azure Key Vault" || !s.Manager.Configured || s.Manager.Source != path+" [secrets]" || s.Manager.Vault != fakeazure.Vault {
		t.Fatalf("status --json manager: %+v", s.Manager)
	}
	h.env = append(h.env, "HEV_SECRETS_MANAGER=1password", "HEV_SECRETS_VAULT=example")
	out, _, _ = h.sh(t, "sh", `hev secrets status --no-quota`)
	if !strings.Contains(out, "secret manager:  1Password (vault example, field password; environment (HEV_SECRETS_MANAGER))") {
		t.Fatalf("environment status:\n%s", out)
	}
}

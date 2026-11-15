package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodConfig = `default = "team"

[notifiers.team]
kind = "slack"
token = "op://example-vault/kit-slack-bot/credential"
audience = "internal"
channels = { builds = "C0123456789", ops = "C0987654321" }

[notifiers.client-a]
kind = "slack"
token = "op://client-a-vault/slack-bot/credential"
audience = "external"
repos = ["https://github.com/ExampleClient/app"]
channels = { delivery = "C0AAAAAAAAA" }
`

// fakeToken is obviously fake and shaped like a bot token, so a leak would
// be caught by the checks below.
const fakeToken = "xo" + "xb-000000000000-000000000000-" + "FAKEFAKEFAKEFAKEFAKEFAKE"

func TestDecodeAcceptsReferences(t *testing.T) {
	c, err := Decode([]byte(goodConfig))
	if err != nil {
		t.Fatal(err)
	}
	if c.Default != "team" || len(c.Notifiers) != 2 {
		t.Fatalf("%+v", c)
	}
	n, err := c.Lookup("")
	if err != nil || n.Name != "team" || n.Channels["builds"] != "C0123456789" || n.External() {
		t.Fatalf("%+v %v", n, err)
	}
	ext := c.Notifiers["client-a"]
	if !ext.External() || ext.AllowsRepo("https://github.com/Other/app") || !ext.AllowsRepo("https://github.com/ExampleClient/app") {
		t.Fatalf("%+v", ext)
	}
	if _, err := c.Lookup("nope"); err == nil || !strings.Contains(err.Error(), "not defined on this node") {
		t.Fatal(err)
	}
}

func TestDecodeIsStrict(t *testing.T) {
	cases := map[string]string{
		"literal token":        strings.Replace(goodConfig, "op://example-vault/kit-slack-bot/credential", fakeToken, 1),
		"https webhook":        strings.Replace(goodConfig, "op://example-vault/kit-slack-bot/credential", "https://hooks.slack.com/services/T000/B000/FAKEFAKEFAKE", 1),
		"unknown top key":      goodConfig + "\nflavour = \"x\"\n",
		"unknown notifier key": strings.Replace(goodConfig, `audience = "internal"`, `audience = "internal"`+"\nwebhook = \"x\"", 1),
		"unsupported kind":     strings.Replace(goodConfig, `kind = "slack"`, `kind = "discord"`, 1),
		"unknown kind":         strings.Replace(goodConfig, `kind = "slack"`, `kind = "irc"`, 1),
		"channel name":         strings.Replace(goodConfig, `"C0123456789"`, `"#builds"`, 1),
		"bad audience":         strings.Replace(goodConfig, `audience = "internal"`, `audience = "public"`, 1),
		"bad default":          strings.Replace(goodConfig, `default = "team"`, `default = "other"`, 1),
		"bad name":             strings.Replace(goodConfig, `[notifiers.team]`, `[notifiers.Team]`, 1),
		"missing token":        strings.Replace(goodConfig, `token = "op://example-vault/kit-slack-bot/credential"`, "", 1),
		"no channels":          strings.Replace(goodConfig, `channels = { delivery = "C0AAAAAAAAA" }`, "", 1),
		"syntax error token":   "[notifiers.team]\nkind = \"slack\"\ntoken = " + fakeToken + "\n",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(text))
			if err == nil {
				t.Fatal("accepted")
			}
			if strings.Contains(err.Error(), "xoxb") || strings.Contains(err.Error(), "FAKE") || strings.Contains(err.Error(), "example-vault") {
				t.Fatalf("error leaks a value: %v", err)
			}
		})
	}
}

func TestPathFollowsTargetConfigOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HEV_DISPATCH_CONFIG", filepath.Join(dir, "dispatch.toml"))
	if Path() != filepath.Join(dir, "notify.toml") {
		t.Fatal(Path())
	}
	if _, err := Load(); err != ErrNoConfig {
		t.Fatalf("missing file: %v", err)
	}
	if err := os.WriteFile(Path(), []byte(goodConfig), 0600); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(); err != nil || len(c.Notifiers) != 2 {
		t.Fatal(c, err)
	}
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if Path() != filepath.Join(home, ".config", "hev", "notify.toml") {
		t.Fatal(Path())
	}
}

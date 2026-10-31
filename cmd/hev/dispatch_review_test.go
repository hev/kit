package main

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
	"github.com/spf13/cobra"
)

func TestAttachTargetWindows(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := dispatchAttach(cmd, dispatch.Host{OS: "windows"}, "sample"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hev dispatch logs sample -f") {
		t.Fatal(out.String())
	}
}

func TestAttachNestedAndRemotePath(t *testing.T) {
	c := attachCommand(context.Background(), dispatch.Host{Local: true}, "sample", true)
	if !reflect.DeepEqual(c.Args, []string{"tmux", "switch-client", "-t", "d-sample"}) {
		t.Fatal(c.Args)
	}
	c = attachCommand(context.Background(), dispatch.Host{SSH: "example", PathPrefix: []string{"~/bin", "/some path/bin"}}, "sample", false)
	line := c.Args[len(c.Args)-1]
	if line != `PATH="$HOME"/'bin':'/some path/bin':"$PATH" tmux attach -t 'd-sample'` {
		t.Fatal(line)
	}
}

func TestBatchPluginRejectsShellSyntax(t *testing.T) {
	for _, arg := range []string{"a&calc", "x|y", "%PATH%", "a!b", "a^b", "a(b)", "x>y", "x<y", "a\"b", "a\nb"} {
		if _, err := cmdPluginLine(`C:\plugin dir\hev-hello.cmd`, []string{arg}); err == nil {
			t.Fatalf("accepted %q", arg)
		}
	}
	line, err := cmdPluginLine(`C:\plugin dir\hev-hello.cmd`, []string{"hello world"})
	if err != nil || line != `""C:\plugin dir\hev-hello.cmd" "hello world""` {
		t.Fatalf("%q %v", line, err)
	}
}

func TestInventoryNameSafety(t *testing.T) {
	used, err := inventoryNames(map[string]dispatch.Response{"remote": {Names: []string{"reserved"}, Rows: []dispatch.Row{{Meta: dispatch.Meta{Order: dispatch.Order{Name: "taken"}}}}}})
	if err != nil || !used["reserved"] || !used["taken"] {
		t.Fatalf("%v %v", used, err)
	}
	if _, err := inventoryNames(map[string]dispatch.Response{"offline": {Error: "unreachable"}}); err == nil {
		t.Fatal("unknown inventory accepted")
	}
}

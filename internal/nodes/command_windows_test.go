//go:build windows

package nodes

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func init() {
	if os.Getenv("HEV_NODES_COMMAND_HELPER") == "1" {
		_ = json.NewEncoder(os.Stdout).Encode(os.Args[1:])
		os.Exit(0)
	}
}

// Exercise the actual default Windows shell with a quoted executable path.
func TestWindowsForcedCommandWithSpaces(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(t.TempDir(), "kit tools")
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(dir, "hev.exe")
	b, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(target, b, 0700); e != nil {
		t.Fatal(e)
	}
	line, e := KeyLine("windows", target, "grant", testKey(t))
	if e != nil {
		t.Fatal(e)
	}
	raw := strings.SplitN(strings.TrimPrefix(line, `restrict,command="`), `" ssh-`, 2)[0]
	command := strings.ReplaceAll(strings.ReplaceAll(raw, `\"`, `"`), `\\`, `\`)
	c := exec.Command("cmd.exe", "/c", command)
	c.Env = append(os.Environ(), "HEV_NODES_COMMAND_HELPER=1")
	out, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("forced command: %v: %s", e, out)
	}
	var args []string
	if e = json.Unmarshal(out, &args); e != nil {
		t.Fatalf("%s: %v", out, e)
	}
	if !reflect.DeepEqual(args, []string{"dispatch", "rpc", "--node", "grant"}) {
		t.Fatal(args)
	}
}

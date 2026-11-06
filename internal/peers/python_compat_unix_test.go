//go:build !windows

package peers

import (
	"encoding/json"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

// The reference is supplied privately rather than vendoring deployment-specific
// source into this repository. No command touches the caller's real state.
func TestPythonInboxInteroperability(t *testing.T) {
	reference := os.Getenv("PEERS_PYTHON_REFERENCE")
	if reference == "" {
		t.Skip("set PEERS_PYTHON_REFERENCE to the reference bin/peers")
	}
	e, call := fixture(t)
	if _, err := call("hello", "inbox", "hold", "--id", "go-message", "--sender", "worker"); err != nil {
		t.Fatal(err)
	}
	ms, err := rows(e.path("desk"))
	if err != nil {
		t.Fatal(err)
	}
	identity := ms[0]["recipient_incarnation"].(string)
	python := func(code string) []byte {
		t.Helper()
		script := `import runpy,sys,json,os
p=runpy.run_path(sys.argv[1],run_name='reference')
g=p['hold'].__globals__
g['STATE_DIR']=sys.argv[2]
g['INBOX']=os.path.join(sys.argv[2],'inbox')
g['incarnation']=lambda *a,**k:sys.argv[3]
g['tmux']=lambda *a,**k:''
` + code
		cmd := exec.Command("python3", "-c", script, reference, e.State, identity)
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("reference: %v: %s", err, b)
		}
		return b
	}
	python(`g['hold']('desk','[held message from worker]','from Python','worker',{'id':'python-message'})`)
	goView, err := e.view("desk", identity)
	if err != nil {
		t.Fatal(err)
	}
	var pyView []Message
	if err := json.Unmarshal(python(`print(json.dumps(g['inbox_view']('desk')[1]))`), &pyView); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(goView, pyView) {
		t.Fatalf("view differs\nGo: %#v\nPython: %#v", goView, pyView)
	}
	if _, err := call("", "inbox", "consume", "python-message", "go-message"); err != nil {
		t.Fatal(err)
	}
	python(`row={'session':'desk'}
g['panes']=lambda **k:[row]
g['recipient_me']=lambda rows:row
g['participates']=lambda row:True
g['pane_incarnation']=lambda row:sys.argv[3]
g['inbox_on']=lambda session:True
g['cmd_inbox'](['ack','go-message','python-message','--session','desk'])`)
	out, err := call("", "inbox", "--peek", "--pending", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if decode(t, out)["unacknowledged"] != float64(0) {
		t.Fatal(out)
	}
	// Reversing the consumer proves Python receipts are accepted by Go.
	goView, err = e.view("desk", identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(python(`print(json.dumps(g['inbox_view']('desk')[1]))`), &pyView); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(goView, pyView) {
		t.Fatal("receipt views differ")
	}
}

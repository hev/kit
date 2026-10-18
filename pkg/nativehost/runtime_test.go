//go:build linux || darwin

package nativehost

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func dependencyPin(t *testing.T, path string) FilePin {
	t.Helper()
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		t.Fatal(e)
	}
	digest, e := fileHash(resolved)
	if e != nil {
		t.Fatal(e)
	}
	return FilePin{resolved, digest}
}
func runtimeFixture(t *testing.T) (fixture, string, string, string, string) {
	t.Helper()
	f := setup(t)
	root := f.p.Runtime.Directory
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Fatal("native Python required", e)
	}
	node, e := exec.LookPath("node")
	if e != nil {
		t.Fatal("native Node required", e)
	}
	// A platform launcher (notably macOS /usr/bin/python3) can delegate to
	// a different base binary. A real venv must declare its actual resolved base.
	base := exec.Command(python, "-c", "import sys; print(sys._base_executable)")
	base.Env = []string{"HOME=" + root, "PATH=/usr/bin:/bin"}
	base.Dir = root
	output, e := base.Output()
	if e != nil {
		t.Fatal("observe native Python base", e)
	}
	pythonPin, nodePin := dependencyPin(t, strings.TrimSpace(string(output))), dependencyPin(t, node)
	envPin := dependencyPin(t, "/usr/bin/env")
	first := filepath.Join(root, "first")
	nodeParent := filepath.Join(root, "node-parent")
	venv := filepath.Join(root, "venv", "bin")
	for _, dir := range []string{first, nodeParent, venv} {
		if e = os.MkdirAll(dir, 0700); e != nil {
			t.Fatal(e)
		}
	}
	// Build a real venv, including pyvenv.cfg and normal Python aliases.
	venvRoot := filepath.Dir(venv)
	c := exec.Command(pythonPin.Path, "-m", "venv", "--without-pip", "--symlinks", venvRoot)
	c.Env = []string{"HOME=" + root, "PATH=/usr/bin:/bin"}
	c.Dir = root
	if output, e := c.CombinedOutput(); e != nil {
		t.Fatalf("create actual native venv: %v %s", e, output)
	}
	actualPython, e := filepath.EvalSymlinks(filepath.Join(venv, "python3"))
	if e != nil || actualPython != pythonPin.Path {
		t.Fatal("venv must alias exact declared interpreter", actualPython, e)
	}
	if e = os.Symlink(nodePin.Path, filepath.Join(nodeParent, "node")); e != nil {
		t.Fatal(e)
	}

	py, js, direct := filepath.Join(root, "assessment.py"), filepath.Join(root, "assessment.js"), filepath.Join(root, "direct.py")
	for name, body := range map[string]string{py: "#!/usr/bin/env python3\nprint('trusted-python')\n", js: "#!/usr/bin/env node\nconsole.log('trusted-node');\n", direct: "#!" + filepath.Join(venv, "python") + "\nprint('trusted-direct')\n"} {
		if e = os.WriteFile(name, []byte(body), 0700); e != nil {
			t.Fatal(e)
		}
	}
	f.p.Runtime.Environment = []string{"HOME=" + root, "PATH=" + first + ":" + nodeParent + ":" + venv + ":/usr/bin:/bin"}
	f.p.Runtime.Files = append(f.p.Runtime.Files, pythonPin, nodePin, envPin, dependencyPin(t, py), dependencyPin(t, js), dependencyPin(t, direct))
	cfg := Config{Version, filepath.Join(root, "control.sock"), []Profile{f.p}}
	b, e := json.Marshal(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(f.config, b, 0600); e != nil {
		t.Fatal(e)
	}
	return f, first, nodeParent, py, js
}
func runRuntimeScript(t *testing.T, p Profile, path, want string) {
	t.Helper()
	c := exec.Command(path)
	c.Env = append([]string(nil), p.Runtime.Environment...)
	c.Dir = p.Runtime.Directory
	b, e := c.CombinedOutput()
	if e != nil || string(b) != want {
		t.Fatalf("owned script %s output %q error %v", filepath.Base(path), b, e)
	}
}
func TestRuntimeNormalVenvAliasesResolveToDeclaredBytes(t *testing.T) {
	f, _, _, py, js := runtimeFixture(t)
	if _, e := VerifyConfig(f.config); e != nil {
		t.Fatal("normal aliases rejected", e)
	}
	runRuntimeScript(t, f.p, py, "trusted-python\n")
	runRuntimeScript(t, f.p, js, "trusted-node\n")
	runRuntimeScript(t, f.p, filepath.Join(f.p.Runtime.Directory, "direct.py"), "trusted-direct\n")
}
func TestRuntimeNodeParentPythonAndNodePathShadowsHoldWithAllPinsUnchanged(t *testing.T) {
	for _, which := range []string{"python3", "node"} {
		t.Run(which, func(t *testing.T) {
			f, first, nodeParent, py, js := runtimeFixture(t)
			before, _ := os.ReadFile(f.config)
			if _, e := VerifyConfig(f.config); e != nil {
				t.Fatal(e)
			}
			shadowDir := first
			script := js
			if which == "python3" {
				shadowDir = nodeParent
				script = py
			}
			marker := filepath.Join(f.p.Runtime.Directory, "shadow-effect")
			shadow := filepath.Join(shadowDir, which)
			if e := os.WriteFile(shadow, []byte("#!/bin/sh\nprintf shadow > '"+marker+"'\nexit 77\n"), 0700); e != nil {
				t.Fatal(e)
			}
			after, _ := os.ReadFile(f.config)
			if !bytes.Equal(before, after) {
				t.Fatal("profile bytes changed")
			}
			for _, pin := range f.p.Runtime.Files {
				if pinned(pin.Path, pin.Digest, false) != nil {
					t.Fatal("declared pin changed")
				}
			}
			if _, e := VerifyConfig(f.config); e == nil {
				t.Fatal("static verifier accepted first PATH shadow")
			}
			if _, e := inspectRegistration(context.Background(), f.p, f.call); e == nil {
				t.Fatal("probe accepted runtime shadow")
			}
			h := &host{profiles: map[string]Profile{f.p.ID: f.p}}
			if e := h.Dispatch(context.Background(), f.call); e == nil {
				t.Fatal("dispatch accepted runtime shadow")
			}
			if _, e := os.Stat(marker); !os.IsNotExist(e) {
				t.Fatal("validation executed script")
			}
			if _, e := os.Stat(filepath.Join(filepath.Dir(f.desc), "host-intent.json")); !os.IsNotExist(e) {
				t.Fatal("shadow created dispatch intent")
			}
			// Demonstrate actual kernel/env resolution of the same unchanged script.
			c := exec.Command(script)
			c.Env = append([]string(nil), f.p.Runtime.Environment...)
			c.Dir = f.p.Runtime.Directory
			e := c.Run()
			exit, ok := e.(*exec.ExitError)
			if !ok || exit.ExitCode() != 77 {
				t.Fatal("OS did not choose shadow", e)
			}
			if b, e := os.ReadFile(marker); e != nil || string(b) != "shadow" {
				t.Fatal("missing owned shadow proof", e)
			}
		})
	}
}
func TestRuntimeRejectsAmbiguousEnvOptionsAndMultiwordKernelArgument(t *testing.T) {
	f, _, _, py, _ := runtimeFixture(t)
	for _, line := range []string{"#!/usr/bin/env python3 -u\n", "#!/usr/bin/env -i python3\n", "#!/usr/bin/env -S ${HOME}\n", "#!/usr/bin/env python3\r\n"} {
		if e := os.WriteFile(py, []byte(line), 0700); e != nil {
			t.Fatal(e)
		}
		for i := range f.p.Runtime.Files {
			if f.p.Runtime.Files[i].Path == py {
				f.p.Runtime.Files[i] = dependencyPin(t, py)
			}
		}
		if f.p.verify() == nil {
			t.Fatal("ambiguous env shebang accepted", line)
		}
	}
}

//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
)

func init() {
	if os.Getenv("HEV_WINDOWS_BRIDGE_HELPER") == "1" {
		data, _ := io.ReadAll(os.Stdin)
		json.NewEncoder(os.Stdout).Encode(struct {
			Args  []string `json:"args"`
			Input string   `json:"input"`
		}{os.Args[1:], string(data)})
		os.Stderr.WriteString("bridge stderr\n")
		os.Exit(23)
	}
}
func TestWindowsResourceBridgePreservesStreamsArgsAndExit(t *testing.T) {
	t.Setenv("HEV_WINDOWS_BRIDGE_HELPER", "1")
	t.Setenv("HEV_RESOURCE_BACKEND", os.Args[0])
	cmd := resourceBackendCommand("get")
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetIn(bytes.NewBufferString("input sentinel\n"))
	args := []string{"nodes", "--selector", "team!=red", "--catalog", `C:\owner path\catalog.json`}
	err := runResourceBackend(cmd, "get", args)
	code, ok := err.(*resourceBackendExit)
	if !ok || code.code != 23 {
		t.Fatal(err)
	}
	var got struct {
		Args  []string `json:"args"`
		Input string   `json:"input"`
	}
	if json.Unmarshal(out.Bytes(), &got) != nil || !reflect.DeepEqual(got.Args, append([]string{"get"}, args...)) || got.Input != "input sentinel\n" || stderr.String() != "bridge stderr\n" {
		t.Fatal(out.String(), stderr.String())
	}
}

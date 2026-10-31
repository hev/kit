package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBatchPluginSpacedPathAndInjection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugin dir")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "hev-hello.cmd")
	if err := os.WriteFile(path, []byte("@echo off\r\necho %~1\r\nexit /b 7\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code, err := runPlugin(path, []string{"hello world"}, strings.NewReader(""), &out, &out)
	if err != nil || code != 7 || strings.TrimSpace(out.String()) != "hello world" {
		t.Fatalf("%d %v %q", code, err, out.String())
	}
	if _, err := runPlugin(path, []string{"a&calc"}, nil, &out, &out); err == nil {
		t.Fatal("unsafe batch argument accepted")
	}
}

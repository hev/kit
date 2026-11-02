package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestFactoryHelpGroups(t *testing.T) {
	root := &cobra.Command{Use: "hev"}
	for _, name := range []string{"up", "d", "s", "dispatch", "peers", "board", "traces", "version", "apply", "get", "describe", "controller", "logs", "watch", "wait", "api-resources"} {
		root.AddCommand(&cobra.Command{Use: name, Short: name, Run: func(*cobra.Command, []string) {}})
	}
	groupFactoryCommands(root)
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	factory, other, legacy := strings.Index(help, "Factory:"), strings.Index(help, "Other commands:"), strings.Index(help, "Legacy:")
	if factory < 0 || other < factory || legacy < other {
		t.Fatalf("wrong help group order:\n%s", help)
	}
	for _, cmd := range root.Commands() {
		if cmd.Hidden {
			t.Errorf("%s unexpectedly hidden", cmd.Name())
		}
		pos := strings.Index(help, "  "+cmd.Name()+" ")
		switch cmd.GroupID {
		case "factory":
			if pos < factory || pos > other {
				t.Errorf("%s outside Factory", cmd.Name())
			}
		case "legacy":
			if pos < legacy {
				t.Errorf("%s outside Legacy", cmd.Name())
			}
		}
	}
}

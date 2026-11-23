package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestClusterHelpGroups(t *testing.T) {
	root := &cobra.Command{Use: "hev"}
	for _, name := range []string{"up", "d", "s", "dispatch", "orchestrate", "steer", "secrets", "cluster", "peers", "board", "traces", "version", "watch"} {
		root.AddCommand(&cobra.Command{Use: name, Short: name, Run: func(*cobra.Command, []string) {}})
	}
	groupClusterCommands(root)
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	help := out.String()
	cluster, other := strings.Index(help, "Cluster:"), strings.Index(help, "Other commands:")
	if cluster < 0 || other < cluster {
		t.Fatalf("wrong help group order:\n%s", help)
	}
	if strings.Contains(help, "Factory:") {
		t.Fatalf("help contains retired group title:\n%s", help)
	}
	for _, cmd := range root.Commands() {
		if cmd.Hidden {
			t.Errorf("%s unexpectedly hidden", cmd.Name())
		}
		pos := strings.Index(help, "  "+cmd.Name()+" ")
		switch cmd.Name() {
		case "version":
			if pos < other {
				t.Errorf("%s outside Other commands", cmd.Name())
			}
		default:
			if pos < cluster || pos > other {
				t.Errorf("%s outside Cluster", cmd.Name())
			}
		}
	}
}

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestDispatchHelpTree(t *testing.T) {
	var walk func(*cobra.Command, []string)
	walk = func(c *cobra.Command, path []string) {
		for _, child := range c.Commands() {
			next := append(append([]string{}, path...), child.Name())
			t.Run(strings.Join(next, "/"), func(t *testing.T) {
				root := &cobra.Command{Use: "hev"}
				root.AddCommand(newDispatchCommand())
				installPluginHelp(root)
				var out bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&out)
				root.SetArgs(append(append([]string{"dispatch"}, next...), "--help"))
				if err := root.Execute(); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out.String(), "hev dispatch "+strings.Join(next, " ")) {
					t.Fatalf("wrong usage: %s", out.String())
				}
				if !strings.Contains(out.String(), "--help") {
					t.Fatalf("missing flags: %s", out.String())
				}
			})
			walk(child, next)
		}
	}
	walk(newDispatchCommand(), nil)
}

func TestPeersHelpDoesNotExecuteBackend(t *testing.T) {
	t.Setenv("HEV_PEERS_BACKEND", "this-backend-must-never-execute")
	addCoreCommands()
	defer func() {
		for _, name := range []string{"nodes", "peers", "traces"} {
			c, _, _ := rootCmd.Find([]string{name})
			rootCmd.RemoveCommand(c)
		}
	}()
	c, _, _ := rootCmd.Find([]string{"peers"})
	for _, flag := range []string{"--help", "-h"} {
		var out bytes.Buffer
		c.SetOut(&out)
		if err := c.RunE(c, []string{flag}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "hev peers") {
			t.Fatal(out.String())
		}
	}
}

func TestSayMissingMessage(t *testing.T) {
	c, _, _ := newDispatchCommand().Find([]string{"say"})
	err := c.Args(c, []string{"example"})
	if err == nil || !strings.Contains(err.Error(), "say") || strings.Contains(err.Error(), "rework") {
		t.Fatalf("%v", err)
	}
}

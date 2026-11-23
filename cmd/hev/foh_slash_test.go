package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/hev/kit/internal/foh"
	"github.com/spf13/cobra"
)

func TestHevSlashDiscovery(t *testing.T) {
	excluded := strings.Fields("serve collect browse kit foh init onboard hook completion help skills-preview")
	// Cover explicit policy names even when a CLI verb is currently an alias or absent.
	root := &cobra.Command{Use: "hev"}
	for _, name := range excluded {
		root.AddCommand(&cobra.Command{Use: name})
	}
	root.AddCommand(&cobra.Command{Use: "hidden", Hidden: true})
	root.AddCommand(&cobra.Command{Use: "future", Short: "New command", GroupID: "cluster"})
	commands := hevSlashCommands(root)
	if len(commands) != 1 || commands[0].Name != "future" ||
		commands[0].Summary != "New command" || commands[0].Group != "cluster" {
		t.Fatalf("discovery registered hidden or excluded commands or omitted metadata: %+v", commands)
	}

	registered := map[string]foh.HevSlashCommand{}
	for _, command := range hevSlashCommands(rootCmd) {
		registered[command.Name] = command
	}
	for _, command := range rootCmd.Commands() {
		_, found := registered[command.Name()]
		want := !command.Hidden && !slices.Contains(excluded, command.Name())
		if found != want {
			t.Errorf("registration of %q = %t, want %t", command.Name(), found, want)
		}
	}
	if registered["review"].Mode != foh.HevSlashTerminal || registered["secrets"].Mode != foh.HevSlashGuarded {
		t.Fatal("review or secrets discovery mode is incorrect")
	}

	for _, name := range strings.Fields("cluster orchestrate traces cron service workspace board s d up down stop pro version open") {
		root.AddCommand(&cobra.Command{Use: name})
	}
	for _, command := range hevSlashCommands(root) {
		var want []string
		switch command.Name {
		case "cluster", "orchestrate", "traces", "cron", "service", "workspace":
			want = []string{"ls"}
		case "board":
			want = []string{"list"}
		}
		if !slices.Equal(command.DefaultArgs, want) {
			t.Errorf("%s defaults = %v, want %v", command.Name, command.DefaultArgs, want)
		}
	}
}

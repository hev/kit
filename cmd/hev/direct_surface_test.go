package main

import "testing"

func TestOnlyCanonicalTraceCommands(t *testing.T) {
	addDirectSurface()
	defer func() {
		for _, n := range []string{"nodes", "orders", "runs", "peers", "traces"} {
			c, _, _ := rootCmd.Find([]string{n})
			rootCmd.RemoveCommand(c)
		}
	}()
	for _, n := range []string{"query", "find", "q", "ls", "trace"} {
		c, _, err := rootCmd.Find([]string{n})
		if err == nil && c != rootCmd {
			t.Fatalf("old command %s remains", n)
		}
	}
	for _, n := range []string{"search", "list", "show"} {
		c, _, err := rootCmd.Find([]string{"traces", n})
		if err != nil || c.RunE == nil {
			t.Fatalf("missing traces %s", n)
		}
	}
}

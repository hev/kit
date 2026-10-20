package main

import (
	"reflect"
	"testing"
)

func TestDirectTraceAliasesShareHandlersAndFlags(t *testing.T) {
	addDirectSurface()
	defer func() {
		for _, name := range []string{"nodes", "orders", "runs", "peers", "traces"} {
			c, _, _ := rootCmd.Find([]string{name})
			rootCmd.RemoveCommand(c)
		}
	}()
	for _, test := range []struct {
		path string
		old  string
		flag string
	}{{"search", "query", "json"}, {"show", "trace", "json"}, {"list", "ls", "since"}} {
		next, _, e := rootCmd.Find([]string{"traces", test.path})
		if e != nil {
			t.Fatal(e)
		}
		old, _, e := rootCmd.Find([]string{test.old})
		if e != nil {
			t.Fatal(e)
		}
		if reflect.ValueOf(next.RunE).Pointer() != reflect.ValueOf(old.RunE).Pointer() {
			t.Fatalf("%s has a competing implementation", test.path)
		}
		if next.Flags().Lookup(test.flag) != old.Flags().Lookup(test.flag) {
			t.Fatalf("%s flag lost compatibility", test.path)
		}
	}
}

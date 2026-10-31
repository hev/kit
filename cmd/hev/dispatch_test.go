package main

import (
	"reflect"
	"testing"
	"time"
)

func TestQueryOrderFilter(t *testing.T) {
	old := queryOrder
	defer func() { queryOrder = old }()
	queryOrder = "example"
	got, e := queryFilter(time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(got, []any{"order", "Eq", "example"}) {
		t.Fatalf("%v", got)
	}
}
func TestDispatchCommandSurface(t *testing.T) {
	cmd := newDispatchCommand()
	for _, name := range []string{"ls", "hosts", "show", "logs", "stop", "rework", "clean", "validate", "supervise", "rpc"} {
		c, _, e := cmd.Find([]string{name})
		if e != nil || c == cmd {
			t.Fatalf("missing %s: %v", name, e)
		}
	}
	for _, name := range []string{"host", "config", "json"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Fatal(name)
		}
	}
}

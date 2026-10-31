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

func TestOrderTimeWindow(t *testing.T) {
	for _, tc := range []struct {
		order         string
		explicit, all bool
		days          int
		want          bool
	}{
		{"example", false, false, 0, false}, {"example", true, false, 0, true},
		{"", false, false, 0, true}, {"example", false, true, 0, true},
	} {
		if got := orderUsesTimeWindow(tc.order, tc.explicit, tc.all, tc.days); got != tc.want {
			t.Fatalf("%+v: %v", tc, got)
		}
	}
}

func TestReview10ReworkNoteCheckedLocally(t *testing.T) {
	cmd := newDispatchCommand()
	rework, _, err := cmd.Find([]string{"rework"})
	if err != nil {
		t.Fatal(err)
	}
	if err = rework.Args(rework, []string{"example"}); err == nil {
		t.Fatal("missing note accepted")
	}
	if err = rework.Flags().Set("from-validation", "true"); err != nil {
		t.Fatal(err)
	}
	if err = rework.Args(rework, []string{"example"}); err != nil {
		t.Fatal(err)
	}
	if cmd.PersistentFlags().Lookup("config").DefValue != "" {
		t.Fatal("environment read outside Load")
	}
}

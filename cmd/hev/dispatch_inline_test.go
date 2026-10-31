package main

import (
	"github.com/spf13/cobra"
	"strings"
	"testing"
)

func TestDispatchStdinTask(t *testing.T) {
	c := &cobra.Command{}
	c.SetIn(strings.NewReader("task from stdin\n"))
	got, err := dispatchMessage(c, []string{"-"})
	if err != nil || got != "task from stdin\n" {
		t.Fatalf("%q %v", got, err)
	}
}

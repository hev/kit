package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hev/kit/internal/dispatch"
)

func TestHetznerDestroyRequiresConfirmation(t *testing.T) {
	host := "cloud"
	c := newDispatchHetzner(func() (dispatch.Config, error) {
		return dispatch.Config{Hosts: map[string]dispatch.Host{"cloud": {Type: "hetzner", Name: "cloud", Mode: "persistent"}}}, nil
	}, &host)
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetErr(&out)
	c.SetArgs([]string{"destroy"})
	if e := c.Execute(); e == nil || !strings.Contains(e.Error(), "--confirm") {
		t.Fatalf("destroy without confirmation: %v", e)
	}
}

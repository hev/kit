package main

import (
	"os"
	"strings"
	"testing"
)

func TestSecretsCleanChildBaseEnv(t *testing.T) {
	t.Setenv("HEV_TEST_ALLOWED", "allowed")
	t.Setenv("HEV_TEST_AMBIENT_SECRET", "fake-secret")
	t.Setenv("OP_SERVICE_ACCOUNT_TOKEN", "fake-token")
	got := secretsChildBaseEnv(true, []string{"HEV_TEST_ALLOWED", "HEV_TEST_ABSENT"})
	if len(got) != 1 || got[0] != "HEV_TEST_ALLOWED=allowed" {
		t.Fatalf("unexpected clean env: %v", got)
	}
	if len(secretsChildBaseEnv(true, nil)) != 0 {
		t.Fatal("clean environment is not empty")
	}
	if strings.Join(secretsChildBaseEnv(false, nil), "\n") != strings.Join(os.Environ(), "\n") {
		t.Fatal("default environment changed")
	}
}

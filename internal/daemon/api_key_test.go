package daemon

import "testing"

func TestAPIKeyEnvironmentBeatsConfig(t *testing.T) {
	kitDaemonHome(t, "[kit]\napi='http://127.0.0.1:1'\nkey='stored-fixture'\n")
	t.Setenv("HEV_API_KEY", "env-fixture")
	k, err := ReadKitConfig()
	if err != nil || k.Key != "env-fixture" {
		t.Fatal("environment key did not win")
	}
}

func TestAPIKeyConfigWhenEnvironmentUnset(t *testing.T) {
	kitDaemonHome(t, "[kit]\napi='http://127.0.0.1:1'\nkey='stored-fixture'\n")
	t.Setenv("HEV_API_KEY", "")
	k, err := ReadKitConfig()
	if err != nil || k.Key != "stored-fixture" {
		t.Fatal("stored key not selected")
	}
}

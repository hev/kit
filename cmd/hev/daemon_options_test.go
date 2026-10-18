package main

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
)

func TestDaemonExplicitRolesPreserveAmbientEnvironment(t *testing.T) {
	t.Setenv("HEV_NATIVE_HOST_CONFIG", "/legacy/profile.json")
	t.Setenv("HEV_CAPTURE_ROLE", "required")
	for _, tc := range []struct {
		args       []string
		path, role string
	}{
		{nil, "/legacy/profile.json", "required"},
		{[]string{"--native-host-config=/unit/profile.json", "--capture-role=held"}, "/unit/profile.json", "held"},
		{[]string{"--native-host-config="}, "", "required"},
	} {
		cmd := &cobra.Command{}
		daemonRoleFlags(cmd)
		if err := cmd.Flags().Parse(tc.args); err != nil {
			t.Fatal(err)
		}
		got, err := daemonRunOptions(cmd)
		if err != nil || got.NativeHostConfig != tc.path || got.CaptureRole != tc.role {
			t.Fatal(got, err)
		}
		if os.Getenv("HEV_NATIVE_HOST_CONFIG") != "/legacy/profile.json" || os.Getenv("HEV_CAPTURE_ROLE") != "required" {
			t.Fatal("unit flags mutated ambient environment")
		}
	}
}

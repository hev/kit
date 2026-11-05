package dispatch

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// A scheduled launch carries the non-secret context that chooses the spec and
// refuses every other difference from the profile, without printing values.
func TestScheduledLaunchEnvironment(t *testing.T) {
	profile := []string{"Path=C:\\Windows", "USERPROFILE=C:\\Users\\u", "HEV_CONFIG=C:\\Users\\u\\hev.toml", "TEMP=C:\\t", "=C:=C:\\"}
	caller := []string{"PATH=C:\\tools;C:\\Windows", "USERPROFILE=C:\\custom", "HOME=C:\\custom", "TEMP=C:\\t", "SSH_CONNECTION=a b c d", "=D:=D:\\"}
	carried, refused := scheduledLaunchEnvironment(caller, profile)
	want := []string{"HEV_CONFIG", "HOME=C:\\custom", "PATH=C:\\tools;C:\\Windows", "USERPROFILE=C:\\custom"}
	if !reflect.DeepEqual(carried, want) || len(refused) != 0 {
		t.Fatalf("carried %q refused %q", carried, refused)
	}
	_, refused = scheduledLaunchEnvironment(append(caller, "GH_TOKEN=synthetic-token", "EXTRA=1"), append(profile, "PROFILE_ONLY=1"))
	if strings.Join(refused, ",") != "EXTRA,GH_TOKEN,PROFILE_ONLY" {
		t.Fatalf("refused %q", refused)
	}
}

func TestApplyLaunchEnvironment(t *testing.T) {
	t.Setenv("HEV_CONFIG", "from-profile")
	t.Setenv("HEV_DISPATCH_CONFIG", "")
	if e := ApplyLaunchEnvironment([]string{"HEV_CONFIG", "HEV_DISPATCH_CONFIG=custom.toml"}); e != nil {
		t.Fatal(e)
	}
	if _, set := os.LookupEnv("HEV_CONFIG"); set || os.Getenv("HEV_DISPATCH_CONFIG") != "custom.toml" {
		t.Fatalf("HEV_CONFIG set %v, HEV_DISPATCH_CONFIG %q", set, os.Getenv("HEV_DISPATCH_CONFIG"))
	}
	if e := ApplyLaunchEnvironment([]string{"GH_TOKEN=x"}); e == nil {
		t.Fatal("launch environment carried a secret")
	}
}

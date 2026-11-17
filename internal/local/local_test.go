package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker puts a `docker` on PATH that logs its arguments and answers
// `docker ps` with ps.
func fakeDocker(t *testing.T, ps string) (log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "docker.log")
	script := `#!/bin/sh
echo "docker $*" >> "` + log + `"
if [ "$1" = ps ]; then
  printf "` + ps + `\n"
fi
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	return log
}

func TestDockerMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := Docker(context.Background()); err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("err = %v", err)
	}
}

func TestStartsOnlyStoppedContainersAndNeverRemoves(t *testing.T) {
	log := fakeDocker(t, `aaa exited gateway\nbbb running postgres\nccc created embed`)
	cs, err := Containers(context.Background(), "hev-local")
	if err != nil || len(cs) != 3 || cs[0].Service != "gateway" || !cs[1].Running {
		t.Fatalf("containers = %+v, %v", cs, err)
	}
	ids, err := Start(context.Background(), cs)
	if err != nil || strings.Join(ids, ",") != "aaa,ccc" {
		t.Fatalf("started = %v, %v", ids, err)
	}
	if err := Stop(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(log)
	got := string(b)
	for _, want := range []string{"label=com.docker.compose.project=hev-local", "docker start aaa ccc", "docker stop aaa ccc"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, " rm") || strings.Contains(got, "volume") || strings.Contains(got, "down") {
		t.Fatalf("removed something:\n%s", got)
	}
}

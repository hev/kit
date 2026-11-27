package dispatch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testWorkID = "0ea82c41-1d54-480e-9dd1-58f13a404df2"

func TestInvalidExplicitWorkID(t *testing.T) {
	for _, id := range []string{"", "garbage", "00000000-0000-0000-0000-000000000000", strings.ToUpper(testWorkID)} {
		order := strings.Replace(good, "name: example", "name: example\nwork_id: "+id, 1)
		if _, err := Parse(order); err == nil || !strings.Contains(err.Error(), "work_id") {
			t.Errorf("order accepted work_id %q: %v", id, err)
		}
		if id != "" {
			if err := assignWorkID(&Meta{WorkID: id}, ""); err == nil {
				t.Errorf("receiver accepted work_id %q", id)
			}
		}
	}
}

func TestLegacyWorkIDRecords(t *testing.T) {
	dir := t.TempDir()
	metaPath, resultPath := filepath.Join(dir, "meta.json"), filepath.Join(dir, "result.json")
	if err := os.WriteFile(metaPath, []byte(`{"name":"old","branch":"d/old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, []byte(`{"branch":"d/old","exit_code":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	var m Meta
	var r Result
	if err := readJSON(metaPath, &m); err != nil || m.WorkID != "" || m.Name != "old" {
		t.Fatalf("legacy meta: %+v, %v", m, err)
	}
	if err := readJSON(resultPath, &r); err != nil || r.WorkID != "" || r.ExitCode != 1 {
		t.Fatalf("legacy result: %+v, %v", r, err)
	}
}

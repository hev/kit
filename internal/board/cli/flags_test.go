package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestFlagErrorsAndHelp(t *testing.T) {
	for _, args := range [][]string{{"ls", "-n=bad"}, {"serve", "--unknown"}} {
		t.Run(args[0]+"Error", func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			old := os.Stderr
			os.Stderr = w
			got := Run(args)
			w.Close()
			os.Stderr = old
			output, _ := io.ReadAll(r)
			r.Close()
			if got == nil {
				t.Fatal("expected error")
			}
			if len(output) != 0 {
				t.Fatalf("flag parser printed error before caller: %q", output)
			}
		})
	}
	for _, cmd := range []string{"ls", "serve"} {
		t.Run(cmd+"Help", func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			old := os.Stdout
			os.Stdout = w
			got := Run([]string{cmd, "--help"})
			w.Close()
			os.Stdout = old
			output, _ := io.ReadAll(r)
			r.Close()
			if got != nil {
				t.Fatal(got)
			}
			if !strings.Contains(string(output), "hev board") {
				t.Fatalf("missing help: %q", output)
			}
		})
	}
}

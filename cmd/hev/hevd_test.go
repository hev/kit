package main

import (
	"bytes"
	"strings"
	"testing"
)

// The text beside hevd starts in one column on every line, whatever the art
// on that line is, and a line past the art keeps the same column.
func TestRenderHevdAlignsText(t *testing.T) {
	lines := []hevdLine{
		{value: "hevd is up"}, {"dashboard", "http://127.0.0.1:8099"}, {"search", "hev find x"},
		{"archive", "hev-traces"}, {"stop", "hev down"}, {"extra", "one past the art"},
	}
	got := strings.Split(strings.Trim(renderHevd(lines), "\n"), "\n")
	if len(got) != len(lines) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(lines), strings.Join(got, "\n"))
	}
	for i, l := range got {
		r := []rune(l)
		want := lines[i].value
		if lines[i].label != "" {
			want = lines[i].label
		}
		if len(r) < hevdWidth || !strings.HasPrefix(string(r[hevdWidth:]), want) {
			t.Fatalf("line %d: text not at column %d: %q", i, hevdWidth, l)
		}
	}
}

// A buffer is not a terminal, so up's output to anything but a TTY carries no art.
func TestPrintHevdSkipsNonTerminal(t *testing.T) {
	var out bytes.Buffer
	printHevd(&out, hevdLine{value: "hevd is up"})
	if out.Len() != 0 {
		t.Fatalf("printed to a buffer: %q", out.String())
	}
}

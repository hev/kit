package store

import "testing"

func TestSnippetMarksWholeWords(t *testing.T) {
	got := snippet("claude -p reports Not logged in. Import the port config.", "port")
	want := "claude -p reports Not logged in. Import the [port] config."
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if got := snippet("nothing in common here at all", "zebra"); got != "nothing in common here at all" {
		t.Errorf("no-match snippet: %q", got)
	}
}

package store

import (
	"testing"

	"github.com/hev/kit/internal/board/layer"
)

func TestAppStoreTakesAnyWellFormedBoard(t *testing.T) {
	app, team := NewApp(nil), New(nil)
	for _, slug := range []string{"story-memory", "job_notes", "a"} {
		if !app.known(slug) {
			t.Errorf("app store refused %q", slug)
		}
		if team.known(slug) && slug == "story-memory" {
			t.Errorf("team board accepted %q", slug)
		}
	}
	for _, slug := range []string{"", "Story", "bad board", "../x", "-lead"} {
		if app.known(slug) {
			t.Errorf("app store accepted %q", slug)
		}
	}
	if !team.known("tooling") {
		t.Error("team board refused tooling")
	}
}

func TestAppTagsAreJSONStringsMatchedInProcess(t *testing.T) {
	rows := []layer.Row{
		{"thread_id": float64(1), "tags": `["job:123","style"]`},
		{"thread_id": float64(2), "tags": `["job:456"]`},
		{"thread_id": float64(3), "tags": []any{"job:123"}}, // array form reads too
	}
	if got := strs(rows[0], "tags"); len(got) != 2 || got[0] != "job:123" {
		t.Fatalf("strs(json string) = %v", got)
	}
	got := withTags(rows, []string{"job:123"})
	if len(got) != 2 || num(got[0], "thread_id") != 1 || num(got[1], "thread_id") != 3 {
		t.Fatalf("withTags(job:123) = %v", got)
	}
	if got := withTags(rows, []string{"job:123", "style"}); len(got) != 1 {
		t.Fatalf("withTags(all of two) = %v", got)
	}
	if got := withTags(rows, nil); len(got) != 3 {
		t.Fatalf("withTags(none) = %v", got)
	}
}

func TestAppStoreReadsAuthorName(t *testing.T) {
	th := threadOf(layer.Row{"thread_id": float64(4), "author": "7001", "author_name": "Ana", "tags": `["job:123"]`, "body": "b"})
	if th.AuthorName != "Ana" || th.Author != "7001" || th.Body() != "b" || len(th.Tags) != 1 {
		t.Fatalf("thread = %+v", th)
	}
	if len(NewApp(nil).threadAttrs()) != len(threadAttrs)+1 || len(New(nil).threadAttrs()) != len(threadAttrs) {
		t.Fatal("only the app store asks for author_name")
	}
}

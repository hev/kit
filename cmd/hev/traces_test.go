package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tracepkg "github.com/hev/kit/internal/trace"
)

func TestFillMissingSummariesSupportsArchivedRows(t *testing.T) {
	rows := []tracepkg.SessionRow{
		{ID: "stored-id", SessionID: "archived", FirstPrompt: "fix the archive"},
		{ID: "titled-id", SessionID: "titled", Summary: "Harness title"},
	}
	calls := 0
	changed, errs := fillMissingSummaries(rows, func(row tracepkg.SessionRow) (string, error) {
		calls++
		if row.FirstPrompt == "" {
			return "", errors.New("missing prompt")
		}
		return "Fix the archive", nil
	})
	if len(errs) != 0 || calls != 1 || len(changed) != 1 {
		t.Fatalf("changed=%+v errors=%v calls=%d", changed, errs, calls)
	}
	if changed[0].ID != "stored-id" || changed[0].Summary != "Fix the archive" {
		t.Fatalf("archive row was not updated in place: %+v", changed[0])
	}
}

func TestFormatModel(t *testing.T) {
	cases := map[string]string{
		"":                          "",
		"claude-opus-4-7":           "Opus 4.7",
		"claude-sonnet-4-6":         "Sonnet 4.6",
		"claude-haiku-4-5-20251001": "Haiku 4.5",
		"gpt-5.5":                   "GPT-5.5",
		"gpt-5":                     "GPT-5",
		"some-model":                "some-model",
	}
	for in, want := range cases {
		if got := formatModel(in); got != want {
			t.Errorf("formatModel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrintAskBlock(t *testing.T) {
	input := json.RawMessage(`{
	  "questions": [
	    {
	      "question": "Which authentication method should we use?",
	      "header": "Auth method",
	      "multiSelect": false,
	      "options": [
	        {"label": "OAuth 2.0", "description": "Industry standard"},
	        {"label": "JWT", "description": "Self-contained tokens"}
	      ]
	    }
	  ]
	}`)
	b := renderBlock{kind: "ask", toolName: "AskUserQuestion", ask: parseAskInput(input)}
	var buf bytes.Buffer
	printAskBlock(b, &buf)
	out := buf.String()
	for _, want := range []string{
		"Asked",
		"AskUserQuestion",
		"Which authentication method",
		"Auth method",
		"single-select",
		"OAuth 2.0",
		"JWT",
		"Industry standard",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n---\n%s", want, out)
		}
	}
}

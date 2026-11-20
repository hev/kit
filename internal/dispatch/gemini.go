package dispatch

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
)

type geminiStreamEvent struct {
	Type    string `json:"type"`
	Session string `json:"session_id"`
	Status  string `json:"status"`
}

// geminiStream reads one attempt's JSONL and ignores diagnostics written to
// the same log. Large tool outputs can be skipped without losing later events.
func geminiStream(path string, offset int64) (session, status string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", ""
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	skipping := false
	for {
		line, more, err := reader.ReadLine()
		if err != nil {
			return session, status
		}
		if skipping || more {
			skipping = more
			continue
		}
		var event geminiStreamEvent
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		switch event.Type {
		case "init":
			if session == "" {
				session = event.Session
			}
		case "result":
			status = event.Status
		}
	}
}

func geminiResultError(status string) error {
	switch status {
	case "success":
		return nil
	case "error":
		return errors.New("Gemini CLI reported an unsuccessful result")
	default:
		return errors.New("Gemini CLI exited without a terminal result")
	}
}

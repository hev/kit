package dispatch

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type cursorResult struct {
	Session string
	Text    *string
}

// readCursorResult reads only the current attempt. Cursor may exit before
// emitting a result, in which case the init event still identifies the session.
func readCursorResult(path string, offset int64) (cursorResult, error) {
	var result cursorResult
	f, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return result, err
	}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 64<<10), 16<<20)
	for scan.Scan() {
		var event struct {
			Type, Subtype string
			Session       string  `json:"session_id"`
			IsError       bool    `json:"is_error"`
			Text          *string `json:"result"`
		}
		if json.Unmarshal(scan.Bytes(), &event) != nil {
			continue
		}
		switch {
		case event.Type == "system" && event.Subtype == "init":
			result.Session = event.Session
		case event.Type == "result":
			if result.Session == "" {
				result.Session = event.Session
			}
			if event.Subtype != "success" || event.IsError || event.Text == nil ||
				event.Session == "" || result.Session != event.Session {
				return result, fmt.Errorf("Cursor emitted an invalid terminal result")
			}
			result.Text = event.Text
			return result, nil
		}
	}
	return result, fmt.Errorf("Cursor stream ended without a successful result")
}

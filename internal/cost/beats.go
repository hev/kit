package cost

import (
	"bufio"
	"encoding/json"
	"os"
)

type Beat struct {
	Instance  string `json:"instance"`
	Session   string `json:"session_id"`
	Timestamp int64  `json:"timestamp"`
}

func ReadBeats(path string) ([]Beat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []Beat{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		var b Beat
		if err := json.Unmarshal(s.Bytes(), &b); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, s.Err()
}

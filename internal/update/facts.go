package update

import (
	"runtime/debug"
	"time"
)

// Observation reads local updater state only. There is no drain lifecycle.
type Observation struct {
	Commit    string
	Staged    bool
	LastCheck time.Time
	Outcome   string
}

func (u *Updater) Observe() (Observation, error) {
	o := Observation{Outcome: "unknown"}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				o.Commit = s.Value[:12]
			}
		}
	}
	s, err := u.LoadState()
	if err != nil {
		return o, err
	}
	o.Staged = s.Pending != nil
	o.LastCheck = s.LastCheck
	if !s.LastCheck.IsZero() {
		o.Outcome = "ok"
		if s.LastError != "" {
			o.Outcome = "failing"
		}
	}
	return o, nil
}

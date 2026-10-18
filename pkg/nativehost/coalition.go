package nativehost

import (
	"encoding/json"
	n "github.com/hev/kit/pkg/nativejob"
	"io"
	"time"
)

const CoalitionVersion = "native-host-coalition/v1"

// CoalitionObservation is a read-only kernel sample, never launch, health,
// membership enumeration, tree-termination or capacity authority.
type CoalitionObservation struct {
	Version      string    `json:"version"`
	HostBirth    n.Birth   `json:"host_birth"`
	ResourceCID  uint64    `json:"resource_cid"`
	TasksStarted uint64    `json:"tasks_started"`
	TasksExited  uint64    `json:"tasks_exited"`
	OSBuild      string    `json:"os_build"`
	ObservedAt   time.Time `json:"observed_at"`
}

// ObserveResourceCoalition binds a sample to an already known live birth.
// expectedCID==0 permits initial observation only. Terminal consumers must
// require the recorded nonzero CID AND the exact original daemon birth.
func ObserveResourceCoalition(expected n.Birth, expectedCID uint64) (CoalitionObservation, error) {
	if expected.PID <= 0 || expected.BootID == "" || expected.Start == "" {
		return CoalitionObservation{}, ErrHeld
	}
	return observeResourceCoalition(expected, expectedCID)
}

// WriteCoalitionObservation is the fixed CLI's strict, bounded metadata codec.
// It does not discover a process, adopt it, or grant any permission.
func WriteCoalitionObservation(input io.Reader, output io.Writer) error {
	data, e := io.ReadAll(io.LimitReader(input, maxFrame+1))
	if e != nil {
		return e
	}
	var request struct {
		Version     string  `json:"version"`
		HostBirth   n.Birth `json:"host_birth"`
		ResourceCID uint64  `json:"resource_cid"`
	}
	if strict(data, &request) != nil || request.Version != CoalitionVersion {
		return ErrHeld
	}
	var members map[string]json.RawMessage
	if json.Unmarshal(data, &members) != nil {
		return ErrHeld
	}
	for _, name := range []string{"version", "host_birth", "resource_cid"} {
		if value, ok := members[name]; ok && string(value) == "null" {
			return ErrHeld
		}
	}
	observation, e := ObserveResourceCoalition(request.HostBirth, request.ResourceCID)
	if e != nil {
		return e
	}
	return json.NewEncoder(output).Encode(observation)
}

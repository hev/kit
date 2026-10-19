package nativehost

import (
	"context"
	"encoding/json"
	n "github.com/hev/kit/pkg/nativejob"
	"io"
	"time"
)

const ServiceVersion = "native-host-service/v1"

// ServiceObservation is a kernel sample, not admission or proof of an idle
// journal. Consumers must independently check originals and resample after pins.
type ServiceObservation struct {
	Version        string    `json:"version"`
	HostBirth      n.Birth   `json:"host_birth"`
	ServiceMembers []n.Birth `json:"service_members"`
	ObservedAt     time.Time `json:"observed_at"`
}

func ObserveService(expectedBirth n.Birth, configPath string) (ServiceObservation, error) {
	if expectedBirth.PID <= 0 || expectedBirth.BootID == "" || expectedBirth.Start == "" || !canonical(configPath) {
		return ServiceObservation{}, ErrHeld
	}
	return platformObserveService(expectedBirth, configPath)
}
func WriteServiceObservation(input io.Reader, output io.Writer) error {
	b, e := io.ReadAll(io.LimitReader(input, maxFrame+1))
	if e != nil {
		return e
	}
	var request struct {
		Version   string  `json:"version"`
		HostBirth n.Birth `json:"host_birth"`
		ConfigRef string  `json:"config_ref"`
	}
	if len(b) > maxFrame || strict(b, &request) != nil || request.Version != ServiceVersion {
		return ErrHeld
	}
	observation, e := ObserveService(request.HostBirth, request.ConfigRef)
	if e != nil {
		return e
	}
	return json.NewEncoder(output).Encode(observation)
}

// ServiceWitness retains query-only references to one authenticated service Job
// and its original host process. Open it before stopping that service. It cannot
// adopt/reopen a PID, signal a process or convert emptiness into result authority.
type ServiceWitness interface {
	Observe() (ServiceObservation, error)
	WaitEmpty(ctx context.Context) error
	Close() error
}

// OpenServiceWitness may return a retained witness together with an error after
// partial setup. The caller must close any nonnil witness, including on error.
// Observe reports historical HostBirth and current members; an empty member set
// is not a claim that an arbitrary process tree or external effects terminated.
func OpenServiceWitness(expectedBirth n.Birth, configPath string) (ServiceWitness, error) {
	if expectedBirth.PID <= 0 || expectedBirth.BootID == "" || expectedBirth.Start == "" || !canonical(configPath) {
		return nil, ErrHeld
	}
	witness, err := platformOpenServiceWitness(expectedBirth, configPath)
	if witness == nil && err == nil {
		return nil, ErrHeld
	}
	return witness, err
}

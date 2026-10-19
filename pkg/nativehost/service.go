package nativehost

import (
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

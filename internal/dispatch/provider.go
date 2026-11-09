package dispatch

import (
	"context"
	"time"
)

// EphemeralProvider is the transport and lifecycle needed by dispatch. Provider
// credentials stay on the dispatching machine and are never part of a record.
type EphemeralProvider interface {
	Create(context.Context, MachineSpec) (Machine, error)
	WaitReady(context.Context, string) error
	Exec(context.Context, string, string) (string, error)
	Copy(context.Context, string, string) ([]byte, error)
	Stop(context.Context, string) error
	Delete(context.Context, string) error
	List(context.Context, string) ([]Machine, error)
	Resume(context.Context, string, time.Duration) error
}
type MachineSpec struct {
	Name, Host, From, Size, Region, Environment string
	Labels                                      map[string]string
	AutoStop                                    time.Duration
}
type Machine struct {
	SSH     string            `json:"ssh,omitempty"`
	HostKey string            `json:"host_key,omitempty"`
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	State   string            `json:"state"`
	Labels  map[string]string `json:"labels"`
}

func (m Machine) live() bool {
	return m.State != "archived" && m.State != "stopped" && m.State != "deleted" && m.State != "cancelled" && m.State != "error"
}

// TransportKind separates provisioners from execution. API-only providers can
// implement exec without pretending to offer an SSH endpoint.
type TransportKind string

const (
	TransportSSH  TransportKind = "ssh"
	TransportExec TransportKind = "exec"
)

type MachineTransport struct {
	Kind               TransportKind
	SSH, HostKey       string
	Port, HostKeyAlias string
}
type CloudProvider interface {
	EphemeralProvider
	Transport(context.Context, string) (MachineTransport, error)
}

func IsCloudHost(h Host) bool { return h.Type == "boat" || h.Type == "command" }

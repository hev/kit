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
	AutoStop                                    time.Duration
}
type Machine struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	State  string            `json:"state"`
	Labels map[string]string `json:"labels"`
}

func (m Machine) live() bool {
	return m.State != "archived" && m.State != "cancelled" && m.State != "error"
}

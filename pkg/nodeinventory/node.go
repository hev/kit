// Package nodeinventory observes this process's native OS without enrolling it or
// inferring executor readiness. The JSON contract is consumed by resource backends.
package nodeinventory

import (
	"context"
	"runtime"
	"time"
)

type Value[T any] struct {
	State  string `json:"state"`
	Value  *T     `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}
type Observation struct {
	Platform     string         `json:"platform"`
	Architecture string         `json:"architecture"`
	Cores        int            `json:"cores"`
	MemoryBytes  Value[uint64]  `json:"memoryBytes"`
	Load1        Value[float64] `json:"load1"`
	Health       Value[string]  `json:"health"`
	Capabilities []string       `json:"capabilities"`
	Capacity     Value[string]  `json:"capacity"`
	CurrentWork  Value[string]  `json:"currentWork"`
	Maintenance  Value[string]  `json:"maintenance"`
	ObservedAt   time.Time      `json:"observedAt"`
	Source       string         `json:"source"`
}

func unknown[T any](reason string) Value[T] { return Value[T]{State: "Unknown", Reason: reason} }

// Observe returns runtime facts only. It does not touch auth, redaction config,
// traces, daemon/job journals, transport configuration or provider credentials.
func Observe(ctx context.Context) (Observation, error) {
	if err := ctx.Err(); err != nil {
		return Observation{}, err
	}
	return Observation{Platform: runtime.GOOS, Architecture: runtime.GOARCH, Cores: runtime.NumCPU(), ObservedAt: time.Now().UTC(), Source: "local-os", Capabilities: []string{"local-os-observation"}, MemoryBytes: unknown[uint64]("memory measurement not implemented by this reader"), Load1: unknown[float64]("load measurement not implemented by this reader"), Health: unknown[string]("executor health authority not configured"), Capacity: unknown[string]("admission authority not configured"), CurrentWork: unknown[string]("execution authority not configured"), Maintenance: unknown[string]("maintenance authority not configured")}, nil
}

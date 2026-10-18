package nativehost

import (
	"context"
	n "github.com/hev/kit/pkg/nativejob"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StartupRegistrationReceipt is an immutable observation, never a launch,
// capacity or health permit. Held observations do not stop capture.
type StartupRegistrationReceipt struct {
	Version       string        `json:"version"`
	ProfileID     string        `json:"profile_id"`
	ProfileDigest string        `json:"profile_digest"`
	HostBirth     n.Birth       `json:"host_birth"`
	State         string        `json:"state"`
	Registration  *Registration `json:"registration,omitempty"`
	ObservedAt    time.Time     `json:"observed_at"`
}

func (h *host) observeStartup(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for _, p := range h.profiles {
		wg.Add(1)
		go func(p Profile) {
			defer wg.Done()
			r := StartupRegistrationReceipt{Version: "native-host-startup/v1", ProfileID: p.ID, ProfileDigest: n.Digest(p), HostBirth: h.birth, State: "Held"}
			select {
			case slots <- struct{}{}:
				registration, err := inspectRegistration(ctx, p, Dispatch{})
				<-slots
				if err == nil {
					r.State = "Observed"
					r.Registration = &registration
				}
			case <-ctx.Done():
			}
			r.ObservedAt = time.Now().UTC()
			key := n.Digest(struct {
				Birth   n.Birth
				Profile string
			}{h.birth, r.ProfileDigest})
			path := filepath.Join(p.StateRoot, "host-registration-"+strings.TrimPrefix(key, "sha256:")+".json")
			err := exclusive(path, r)
			h.mu.Lock()
			h.registered[p.ID] = err == nil && r.State == "Observed"
			h.mu.Unlock()
		}(p)
	}
	wg.Wait()
}

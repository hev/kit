package secrets

import (
	"context"
	"testing"
	"time"
)

func TestMeterDefaultOffAndNonMeterHost(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	ctx := context.Background()
	c.scheduledMeter(ctx)
	setPolicy(t, Policy{Daily: 100, Hourly: 40, CallerMinute: 5, MeterInterval: "1s"})
	c.scheduledMeter(ctx)
	if len(calls()) != 0 {
		t.Fatalf("non-meter host made requests: %v", calls())
	}
}

func TestMeterDesignatedIntervalAndSavedAge(t *testing.T) {
	for _, interval := range []string{"", "3h"} {
		t.Run("interval="+interval, func(t *testing.T) {
			c := fake(t)
			calls := fakeCalls(t)
			t.Setenv("HEV_FAKE_ARRAY", "1")
			setPolicy(t, Policy{Daily: 100, Hourly: 40, CallerMinute: 5, MeterHost: true, MeterInterval: interval})
			now := time.Now()
			c.now = func() time.Time { return now }
			ctx := context.Background()
			c.scheduledMeter(ctx)
			if len(calls()) != 1 {
				t.Fatalf("initial calls: %v", calls())
			}
			step := 2 * time.Hour
			if interval != "" {
				step = 3 * time.Hour
			}
			now = now.Add(step - time.Second)
			c.scheduledMeter(ctx)
			if len(calls()) != 1 {
				t.Fatalf("early poll: %v", calls())
			}
			restarted := New()
			restarted.op = c.op
			restarted.now = c.now
			s := restarted.LocalStatus()
			if len(s.Quota) != 2 || s.QuotaAgeSeconds != int64((step-time.Second).Seconds()) {
				t.Fatalf("stale saved status: %+v", s)
			}
			m := SavedMeter(now)
			if m == nil || m.AgeSeconds != s.QuotaAgeSeconds || len(m.Quota) != 2 {
				t.Fatalf("saved attribution meter: %+v", m)
			}
			restarted.scheduledMeter(ctx)
			if len(calls()) != 1 {
				t.Fatal("restart polled early")
			}
			now = now.Add(time.Second)
			restarted.scheduledMeter(ctx)
			if len(calls()) != 2 {
				t.Fatalf("due poll: %v", calls())
			}
			setPolicy(t, Policy{Daily: 100, Hourly: 40, CallerMinute: 5})
			now = now.Add(24 * time.Hour)
			restarted.scheduledMeter(ctx)
			if len(calls()) != 2 {
				t.Fatal("disabled host still polled")
			}
		})
	}
}

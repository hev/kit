package cluster

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestCronCooldownFactsAreBoundedAndExpire(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	c := newCounters("fixture", true)
	c.now = func() time.Time { return now }
	until := now.Add(time.Minute)
	key := func(i int) string { return fmt.Sprintf("%032x", i) }
	for i := range jobLimit {
		if cooling, err := c.ReserveCronCooldown(key(i), until); err != nil || cooling {
			t.Fatalf("could not reserve %d: %v", i, err)
		}
	}
	if cooling, err := c.ReserveCronCooldown(key(0), until.Add(time.Minute)); err != nil || !cooling {
		t.Fatal("an active deadline must prevent a later reservation")
	}
	if _, err := c.ReserveCronCooldown(key(jobLimit), until); err == nil {
		t.Fatal("the deadline map must have a hard cap")
	}
	invalid, _ := json.Marshal(counterState{Version: 1, CronDeadlines: map[string]time.Time{
		key(jobLimit): until, "local path must be refused": until,
	}})
	if c.merge(invalid) == nil {
		t.Fatal("only fixed-length digest keys may be merged")
	}
	replica := newCounters("replica", true)
	replica.now = c.now
	if err := replica.merge(c.snapshot()); err != nil {
		t.Fatal(err)
	}
	if cooling, err := replica.ReserveCronCooldown(key(0), until); err != nil || !cooling {
		t.Fatal("a replica must honor the deadline")
	}
	now = until
	if cooling, err := replica.ReserveCronCooldown(key(jobLimit), now.Add(time.Minute)); err != nil || cooling || len(replica.state.CronDeadlines) != 1 {
		t.Fatal("expired deadlines must be pruned and free capacity")
	}
}

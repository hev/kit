package secrets

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"
)

type capturedUsage struct {
	ClusterUsage
	account AccountStatus
	meter   SharedMeter
}

func (u *capturedUsage) PublishUsage(map[string]map[string]uint64, Policy, time.Time) error {
	return nil
}
func (u *capturedUsage) Usage(string, time.Time) AccountStatus   { return u.account }
func (u *capturedUsage) PeerCooldown(string) (time.Time, string) { return time.Time{}, "" }
func (u *capturedUsage) SharedMeter() SharedMeter                { return SharedMeter{} }
func (u *capturedUsage) PublishMeter(m SharedMeter)              { u.meter = m }

func TestAccountMeterRefusalAndSaturatedTotals(t *testing.T) {
	c := fake(t)
	calls := fakeCalls(t)
	setPolicy(t, Policy{Daily: 30, Hourly: 30, CallerMinute: 30, AccountDaily: map[string]int{"op": 1}})
	u := &capturedUsage{account: AccountStatus{Visible: true, UsedDay: math.MaxUint64}}
	c.AttachCluster(u)
	if e := c.reserve(time.Now(), "worker", "op", 1); !IsBudgetError(e) || !strings.Contains(e.Error(), "account daily") {
		t.Fatalf("saturated total bypassed budget: %v", e)
	}
	s := c.Status(context.Background())
	if !strings.Contains(s.QuotaUnavailable, "account daily") {
		t.Fatalf("meter mislabels account refusal: %s", s.QuotaUnavailable)
	}
	if len(calls()) != 0 {
		t.Fatal("refused meter executed provider")
	}
}
func TestSharedMeterDerivesMissingNumbersOnlyOnMeterHost(t *testing.T) {
	c := fake(t)
	u := &capturedUsage{}
	c.AttachCluster(u)
	limit, remaining := int64(1000), int64(250)
	reading := MeterReading{CheckedAt: time.Now(), Quota: []QuotaRow{{Type: "account", Limit: &limit, Remaining: &remaining}}}
	setPolicy(t, Policy{Daily: 200, Hourly: 40, CallerMinute: 5})
	c.publishMeterReading(reading)
	if !u.meter.Taken.IsZero() {
		t.Fatal("non-meter host published")
	}
	setPolicy(t, Policy{Daily: 200, Hourly: 40, CallerMinute: 5, MeterHost: true})
	c.publishMeterReading(reading)
	if u.meter.Used != 750 || u.meter.Remaining != 250 {
		t.Fatalf("missing used: %+v", u.meter)
	}
	used := int64(800)
	reading.Quota[0].Used = &used
	reading.Quota[0].Remaining = nil
	c.publishMeterReading(reading)
	if u.meter.Remaining != 200 {
		t.Fatalf("missing remaining: %+v", u.meter)
	}
}

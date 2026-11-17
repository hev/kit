package secrets

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/hev/kit/internal/platformops"
)

// MeterReading contains only sanitized numeric allowances and known row labels.
type MeterReading struct {
	CheckedAt  time.Time  `json:"checked_at"`
	AgeSeconds int64      `json:"age_seconds"`
	Quota      []QuotaRow `json:"quota"`
}

type meterState struct {
	Account   string       `json:"account"`
	AttemptAt time.Time    `json:"attempt_at"`
	Flat      string       `json:"flat"`
	Reading   MeterReading `json:"reading"`
}

func meterPath() string { return filepath.Join(policyDir(), "meter.json") }

func readMeter() *meterState {
	b, err := os.ReadFile(meterPath())
	if err != nil {
		return nil
	}
	var m meterState
	if json.Unmarshal(b, &m) != nil || m.Account != accountID() {
		return nil
	}
	return &m
}

// SavedMeter reads local metadata only, including on hosts without an endpoint.
func SavedMeter(now time.Time) *MeterReading {
	m := readMeter()
	if m == nil || m.Reading.CheckedAt.IsZero() {
		return nil
	}
	m.Reading.AgeSeconds = max(0, int64(now.Sub(m.Reading.CheckedAt).Seconds()))
	return &m.Reading
}

func (c *Cache) loadMeter() {
	if c.clusterUsage() != nil {
		m := c.clusterUsage().SharedMeter()
		c.mu.Lock()
		if m.Taken.After(c.quotaSuccessAt) {
			limit, used, remaining := int64(m.Limit), int64(m.Used), int64(m.Remaining)
			c.quotaSuccessAt = m.Taken
			c.quota = []QuotaRow{{Type: "account", Action: "read_write", Limit: &limit, Used: &used, Remaining: &remaining}}
		}
		c.mu.Unlock()
	}
	m := readMeter()
	if m == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if m.AttemptAt.After(c.quotaAt) {
		c.quotaAt = m.AttemptAt
	}
	if m.Reading.CheckedAt.After(c.quotaSuccessAt) {
		c.quotaSuccessAt = m.Reading.CheckedAt
		c.quota = m.Reading.Quota
		c.quotaFlat = m.Flat
	}
}

func (c *Cache) saveMeter() {
	c.mu.Lock()
	m := meterState{Account: accountID(), AttemptAt: c.quotaAt, Flat: c.quotaFlat,
		Reading: MeterReading{CheckedAt: c.quotaSuccessAt, Quota: append([]QuotaRow(nil), c.quota...)}}
	c.mu.Unlock()
	c.publishMeterReading(m.Reading)
	b, err := json.Marshal(m)
	if err == nil {
		err = platformops.WriteFileAtomic(meterPath(), b, 0600)
	}
	if err != nil {
		c.mu.Lock()
		c.quotaError = "cannot persist meter reading"
		c.mu.Unlock()
	}
}

// Policy is exclusively host-local. Other hosts still evaluate free host warnings.
func (c *Cache) scheduledMeter(ctx context.Context) {
	c.warnings(ctx, c.LocalStatus())
	p, err := hostPolicy()
	if err != nil || !p.MeterHost {
		return
	}
	interval := 2 * time.Hour
	if p.MeterInterval != "" {
		interval, _ = time.ParseDuration(p.MeterInterval)
	}
	c.mu.Lock()
	due := c.quotaAt.IsZero() || c.now().Sub(c.quotaAt) >= interval
	c.mu.Unlock()
	if due {
		c.Status(ctx)
	}
}

func (c *Cache) publishMeterReading(reading MeterReading) {
	p, err := hostPolicy()
	if err != nil || !p.MeterHost {
		return
	}
	if c.clusterUsage() != nil {
		for _, q := range reading.Quota {
			if q.Type != "account" || q.Limit == nil || *q.Limit < 0 {
				continue
			}
			used, remaining := int64(-1), int64(-1)
			if q.Used != nil {
				used = *q.Used
			}
			if q.Remaining != nil {
				remaining = *q.Remaining
			}
			if q.Used == nil && remaining >= 0 && remaining <= *q.Limit {
				used = *q.Limit - remaining
			}
			if q.Remaining == nil && used >= 0 {
				remaining = max(0, *q.Limit-used)
			}
			if used >= 0 && remaining >= 0 {
				c.clusterUsage().PublishMeter(SharedMeter{Limit: uint64(*q.Limit), Used: uint64(used), Remaining: uint64(remaining), Taken: reading.CheckedAt})
			}
		}
	}
}

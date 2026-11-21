package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"

	"github.com/hev/kit/internal/cluster"
)

// Membership provides access to the local SWIM view and advisory cooldowns.
type Membership struct {
	Self    string
	Enabled bool
	Members func(context.Context) []cluster.Member
	Reserve func(context.Context, string, time.Time) (bool, error)
}

func (m Membership) winner(ctx context.Context, j Job) (winner, reason string) {
	if len(j.Placement) == 0 {
		return "", ""
	}
	if !m.Enabled {
		if len(j.Placement) > 1 {
			return "", "multi-host placement requires an active cluster"
		}
		if j.Placement[0] != m.Self {
			return "", "local node is outside placement"
		}
		return m.Self, ""
	}
	var members []cluster.Member
	if m.Members != nil {
		members = m.Members(ctx)
	}
	var highest [32]byte
	for _, member := range members {
		if member.State != "alive" || !slices.Contains(j.Placement, member.Name) {
			continue
		}
		score := sha256.Sum256([]byte(j.Name + "\x00" + member.Name))
		if winner == "" || bytes.Compare(score[:], highest[:]) > 0 ||
			(score == highest && member.Name > winner) {
			winner, highest = member.Name, score
		}
	}
	switch {
	case !slices.Contains(j.Placement, m.Self):
		return winner, "local node is outside placement"
	case winner == "":
		return "", "no alive placement member in the local cluster view"
	case winner != m.Self:
		return winner, "another member won placement"
	default:
		return winner, ""
	}
}

func (m Membership) cooling(ctx context.Context, j Job, start time.Time) (bool, error) {
	if !m.Enabled || len(j.Placement) == 0 || j.cooldown == 0 {
		return false, nil
	}
	if m.Reserve == nil {
		return false, cluster.ErrBound
	}
	members := slices.Clone(j.Placement)
	slices.Sort(members)
	key := sha256.New()
	key.Write([]byte(j.Name))
	for _, member := range members {
		key.Write([]byte("\x00" + member))
	}
	return m.Reserve(ctx, hex.EncodeToString(key.Sum(nil)[:16]), start.Add(j.cooldown))
}

// RemoteMembership uses the daemon's same-user cluster endpoint for manual jobs.
func RemoteMembership(self string, enabled bool) Membership {
	return Membership{
		Self: self, Enabled: enabled,
		Members: func(ctx context.Context) []cluster.Member {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			r, _ := cluster.Query(ctx, cluster.Request{Action: "job-members"})
			return r.Members
		},
		Reserve: func(ctx context.Context, key string, until time.Time) (bool, error) {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			r, err := cluster.Query(ctx, cluster.Request{Action: "reserve-job-cooldown", JobKey: key, JobUntil: &until})
			return r.JobCooling, err
		},
	}
}

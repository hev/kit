package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"
	"time"

	"github.com/hev/kit/internal/cluster"
)

// Membership provides access to the local SWIM view for active-standby placement.
type Membership struct {
	Self    string
	Enabled bool
	Members func(context.Context) []cluster.Member
}

func (m Membership) winner(ctx context.Context, s Service) (winner, reason string) {
	if len(s.Placement) == 0 {
		return "", ""
	}
	if !m.Enabled {
		if len(s.Placement) > 1 {
			return "", "multi-host placement requires an active cluster"
		}
		if s.Placement[0] != m.Self {
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
		if member.State != "alive" || !slices.Contains(s.Placement, member.Name) {
			continue
		}
		score := sha256.Sum256([]byte(s.Name + "\x00" + member.Name))
		if winner == "" || bytes.Compare(score[:], highest[:]) > 0 ||
			(score == highest && member.Name > winner) {
			winner, highest = member.Name, score
		}
	}
	switch {
	case !slices.Contains(s.Placement, m.Self):
		return winner, "local node is outside placement"
	case winner == "":
		return "", "no alive placement member in the local cluster view"
	case winner != m.Self:
		return winner, "another member won placement"
	default:
		return winner, ""
	}
}

// RemoteMembership queries the daemon's same-user cluster endpoint for CLI inspection.
func RemoteMembership(self string, enabled bool) Membership {
	return Membership{
		Self:    self,
		Enabled: enabled,
		Members: func(ctx context.Context) []cluster.Member {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			r, _ := cluster.Query(ctx, cluster.Request{Action: "service-members"})
			return r.Members
		},
	}
}

package daemon

import (
	"reflect"
	"testing"

	"github.com/hev/kit/internal/peers/compat"
)

func TestPeerPresenceTransitions(t *testing.T) {
	peer := compat.LocalPeer{Address: "desk", Agent: "codex"}
	for _, tc := range []struct {
		name, before, after string
		want                []string
	}{
		{"baseline", "unsampled", "%1:10", nil},
		{"appeared", "", "%1:10", []string{"peer-online"}},
		{"ended", "%1:10", "", []string{"peer-offline"}},
		{"unchanged", "%1:10", "%1:10", nil},
		{"replaced", "%1:10", "%1:11", []string{"peer-offline", "peer-online"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old, current := map[string]compat.LocalPeer{}, map[string]compat.LocalPeer{}
			if tc.before == "unsampled" {
				old = nil
			} else if tc.before != "" {
				old[tc.before] = peer
			}
			if tc.after != "" {
				current[tc.after] = peer
			}
			var got []string
			peerTransitions(old, current, func(event string, actual compat.LocalPeer) {
				if actual != peer {
					t.Errorf("peer = %+v, want %+v", actual, peer)
				}
				got = append(got, event)
			})
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("events = %v, want %v", got, tc.want)
			}
		})
	}
}

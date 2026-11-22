package metrics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math"
	"slices"
	"sort"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/secrets"
	"github.com/hev/kit/internal/version"
)

const maxClusterMembers = 64

// Sources supplies read-only, non-blocking local views into hevd subsystems.
// No source may return secret values, local paths, or perform network/provider calls.
type Sources struct {
	Self             string
	LocalVersion     string
	ClusterEnabled   bool
	Members          func(context.Context) []cluster.Member
	PlacementMembers func(context.Context) []cluster.Member
	SecretsStatus    func() *secrets.Status
	Orchestrators    func() ([]dispatch.OrchestratorState, error)
	CronFailing      func(context.Context) (int, error)
}

// EmitClusterAllowed reports whether this node should emit cluster-wide
// hev.cluster.* gauges given the configured placement list and current SWIM view.
// An empty placement always emits locally. A non-empty placement elects a single
// alive witness by SHA-256 rendezvous hash of "metrics\x00" + member.Name.
func EmitClusterAllowed(placement []string, self string, clusterEnabled bool, members []cluster.Member) bool {
	if len(placement) == 0 {
		return true
	}
	if !clusterEnabled {
		return len(placement) == 1 && placement[0] == self
	}
	if !slices.Contains(placement, self) {
		return false
	}
	var winner string
	var highest [32]byte
	for _, m := range members {
		if m.State != "alive" || !slices.Contains(placement, m.Name) {
			continue
		}
		score := sha256.Sum256([]byte("metrics\x00" + m.Name))
		if winner == "" || bytes.Compare(score[:], highest[:]) > 0 ||
			(score == highest && m.Name > winner) {
			winner, highest = m.Name, score
		}
	}
	return winner != "" && winner == self
}

// Collect gathers a validated snapshot of all allowlisted cluster-wide and
// host-local gauges.
func Collect(ctx context.Context, cfg Config, src Sources) []Sample {
	self := NormalizeSelf(src.Self)
	localVer := src.LocalVersion
	if localVer == "" {
		localVer = version.Version
	}

	var out []Sample
	add := func(name string, tags map[string]string, value float64) {
		s := Sample{Name: name, Tags: tags, Value: value}
		if _, _, err := ValidateSample(s); err == nil {
			out = append(out, s)
		}
	}

	// Cluster-wide gauges (gated by optional placement witness election).
	if src.Members != nil {
		placementMembers := src.Members(ctx)
		if src.PlacementMembers != nil {
			placementMembers = src.PlacementMembers(ctx)
		}
		if EmitClusterAllowed(cfg.Placement, src.Self, src.ClusterEnabled, placementMembers) {
			members := src.Members(ctx)
			if len(members) > 0 {
				sorted := slices.Clone(members)
				sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
				if len(sorted) > maxClusterMembers {
					sorted = sorted[:maxClusterMembers]
				}
				refVersion := referenceVersion(sorted, localVer)
				for _, m := range sorted {
					if !ValidMemberName(m.Name) {
						continue
					}
					tags := map[string]string{"member": m.Name, "source": "swim"}
					alive := 0.0
					if m.State == "alive" {
						alive = 1.0
					}
					add(MetricClusterMemberAlive, tags, alive)

					skew := 0.0
					if m.Version != refVersion {
						skew = 1.0
					}
					add(MetricClusterVersionSkew, tags, skew)

					if m.State == "alive" && m.Activity != nil {
						if m.Activity.HeadlessRuns != nil && *m.Activity.HeadlessRuns >= 0 {
							add(MetricClusterHeadlessRuns, tags, float64(*m.Activity.HeadlessRuns))
						}
						if m.Activity.Orchestrators != nil && *m.Activity.Orchestrators >= 0 {
							add(MetricClusterOrchestrators, tags, float64(*m.Activity.Orchestrators))
						}
						if m.Activity.AgentsWorking != nil && *m.Activity.AgentsWorking >= 0 {
							add(MetricClusterAgentsWorking, tags, float64(*m.Activity.AgentsWorking))
						}
						if m.Activity.AgentsIdle != nil && *m.Activity.AgentsIdle >= 0 {
							add(MetricClusterAgentsIdle, tags, float64(*m.Activity.AgentsIdle))
						}
					}
				}
			}
		}
	}

	// Host-local 1Password / secrets budget gauges (always local, zero op calls).
	if src.SecretsStatus != nil {
		if st := src.SecretsStatus(); st != nil && st.Budget.Error == "" {
			localTags := map[string]string{"member": self}
			usedHour := max(0, st.Budget.UsedHour)
			usedDay := max(0, st.Budget.UsedDay)
			add(MetricSecretsOPUsedHourly, localTags, float64(usedHour))
			add(MetricSecretsOPHourlyPct, localTags, percentage(usedHour, st.Budget.Hourly))
			add(MetricSecretsOPUsedDaily, localTags, float64(usedDay))
			add(MetricSecretsOPDailyPct, localTags, percentage(usedDay, st.Budget.Daily))
		}
	}

	// Host-local orchestrator counts by allowlisted state.
	if src.Orchestrators != nil {
		if states, err := src.Orchestrators(); err == nil {
			counts := make(map[string]int, len(OrchestratorStates))
			for _, st := range states {
				if mapped := mapOrchestratorState(st.State); mapped != "" {
					counts[mapped]++
				}
			}
			for _, state := range OrchestratorStates {
				add(MetricOrchestratorByState, map[string]string{"member": self, "state": state}, float64(counts[state]))
			}
		}
	}

	// Host-local failing cron jobs count.
	if src.CronFailing != nil {
		if failing, err := src.CronFailing(ctx); err == nil && failing >= 0 {
			add(MetricCronFailing, map[string]string{"member": self}, float64(failing))
		}
	}

	return out
}

// referenceVersion picks the modal version among alive SWIM members, breaking
// ties in favor of localVersion, then lexicographically highest version string.
func referenceVersion(members []cluster.Member, localVersion string) string {
	counts := map[string]int{}
	for _, m := range members {
		if m.State == "alive" && m.Version != "" {
			counts[m.Version]++
		}
	}
	if len(counts) == 0 {
		return localVersion
	}
	bestVer := ""
	bestCount := -1
	for ver, c := range counts {
		if c > bestCount ||
			(c == bestCount && ver == localVersion) ||
			(c == bestCount && bestVer != localVersion && ver > bestVer) {
			bestVer, bestCount = ver, c
		}
	}
	return bestVer
}

func mapOrchestratorState(state string) string {
	switch state {
	case "running":
		return "running"
	case "waiting", "ready", "idle":
		return "waiting"
	case "failed":
		return "failed"
	case "stalled":
		return "stalled"
	case "broken":
		return "broken"
	default:
		return ""
	}
}

func percentage(used, limit int) float64 {
	if limit <= 0 || used <= 0 {
		return 0
	}
	return math.Round(10000*float64(used)/float64(limit)) / 100
}

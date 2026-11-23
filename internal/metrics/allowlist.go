package metrics

import (
	"errors"
	"math"
)

// Allowlisted metric names (DogStatsD dotted form).
const (
	MetricClusterMemberAlive   = "hev.cluster.member_alive"
	MetricClusterVersionSkew   = "hev.cluster.version_skew"
	MetricClusterHeadlessRuns  = "hev.cluster.headless_runs"
	MetricClusterOrchestrators = "hev.cluster.orchestrators"
	MetricClusterAgentsWorking = "hev.cluster.agents_working"
	MetricClusterAgentsIdle    = "hev.cluster.agents_idle"

	MetricSecretsOPUsedHourly = "hev.secrets.op_used_hourly"
	MetricSecretsOPHourlyPct  = "hev.secrets.op_hourly_pct"
	MetricSecretsOPUsedDaily  = "hev.secrets.op_used_daily"
	MetricSecretsOPDailyPct   = "hev.secrets.op_daily_pct"

	MetricOrchestratorByState = "hev.orchestrator.by_state"
	MetricCronFailing         = "hev.cron.failing"
)

// OrchestratorStates is the fixed, ordered allowlist of values for the "state"
// tag on hev.orchestrator.by_state.
var OrchestratorStates = []string{"running", "waiting", "failed", "stalled", "broken"}

var allowedOrchestratorStates = map[string]bool{
	"running": true,
	"waiting": true,
	"failed":  true,
	"stalled": true,
	"broken":  true,
}

// MetricDef describes one allowlisted gauge.
type MetricDef struct {
	DogStatsD  string
	Prometheus string
	Help       string
	Tags       []string
}

// AllowlistOrder defines the canonical emission order of all 12 allowlisted metrics.
var AllowlistOrder = []MetricDef{
	{
		DogStatsD:  MetricClusterMemberAlive,
		Prometheus: "hev_cluster_member_alive",
		Help:       "Whether a SWIM cluster member is alive (1) or not alive (0).",
		Tags:       []string{"member", "source"},
	},
	{
		DogStatsD:  MetricClusterVersionSkew,
		Prometheus: "hev_cluster_version_skew",
		Help:       "Whether a SWIM cluster member version differs from the cluster reference version (1) or matches (0).",
		Tags:       []string{"member", "source"},
	},
	{
		DogStatsD:  MetricClusterHeadlessRuns,
		Prometheus: "hev_cluster_headless_runs",
		Help:       "Active headless dispatch runs reported by a SWIM cluster member.",
		Tags:       []string{"member", "source"},
	},
	{
		DogStatsD:  MetricClusterOrchestrators,
		Prometheus: "hev_cluster_orchestrators",
		Help:       "Live orchestrator supervisors reported by a SWIM cluster member.",
		Tags:       []string{"member", "source"},
	},
	{
		DogStatsD:  MetricClusterAgentsWorking,
		Prometheus: "hev_cluster_agents_working",
		Help:       "Working agent panes reported by a SWIM cluster member.",
		Tags:       []string{"member", "source"},
	},
	{
		DogStatsD:  MetricClusterAgentsIdle,
		Prometheus: "hev_cluster_agents_idle",
		Help:       "Idle agent panes reported by a SWIM cluster member.",
		Tags:       []string{"member", "source"},
	},
	{
		DogStatsD:  MetricSecretsOPUsedHourly,
		Prometheus: "hev_secrets_op_used_hourly",
		Help:       "Local host secrets budget requests used in the rolling hour.",
		Tags:       []string{"member"},
	},
	{
		DogStatsD:  MetricSecretsOPHourlyPct,
		Prometheus: "hev_secrets_op_hourly_pct",
		Help:       "Percentage of the local host hourly secrets budget used.",
		Tags:       []string{"member"},
	},
	{
		DogStatsD:  MetricSecretsOPUsedDaily,
		Prometheus: "hev_secrets_op_used_daily",
		Help:       "Local host secrets budget requests used in the rolling 24-hour window.",
		Tags:       []string{"member"},
	},
	{
		DogStatsD:  MetricSecretsOPDailyPct,
		Prometheus: "hev_secrets_op_daily_pct",
		Help:       "Percentage of the local host daily secrets budget used.",
		Tags:       []string{"member"},
	},
	{
		DogStatsD:  MetricOrchestratorByState,
		Prometheus: "hev_orchestrator_by_state",
		Help:       "Count of local orchestrators by allowlisted state.",
		Tags:       []string{"member", "state"},
	},
	{
		DogStatsD:  MetricCronFailing,
		Prometheus: "hev_cron_failing",
		Help:       "Count of enabled, unpaused, locally eligible cron jobs with consecutive failures.",
		Tags:       []string{"member"},
	},
}

var allowlistByName = func() map[string]MetricDef {
	m := make(map[string]MetricDef, len(AllowlistOrder))
	for _, def := range AllowlistOrder {
		m[def.DogStatsD] = def
	}
	return m
}()

// Tag is a validated key/value label pair.
type Tag struct {
	Key   string
	Value string
}

// Sample is a single gauge observation prior to allowlist verification.
type Sample struct {
	Name  string
	Tags  map[string]string
	Value float64
}

// ValidateSample enforces the strict metric name, tag key, tag value, and
// finite numeric allowlist. Any non-allowlisted input is refused.
func ValidateSample(s Sample) (MetricDef, []Tag, error) {
	def, ok := allowlistByName[s.Name]
	if !ok {
		return MetricDef{}, nil, errors.New("metric name is not allowlisted")
	}
	if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
		return MetricDef{}, nil, errors.New("metric value must be finite")
	}
	if len(s.Tags) != len(def.Tags) {
		return MetricDef{}, nil, errors.New("metric tag set does not match allowlist")
	}
	tags := make([]Tag, 0, len(def.Tags))
	for _, key := range def.Tags {
		val, present := s.Tags[key]
		if !present {
			return MetricDef{}, nil, errors.New("missing allowlisted metric tag")
		}
		if !validTagValue(key, val) {
			return MetricDef{}, nil, errors.New("metric tag value is not allowlisted")
		}
		tags = append(tags, Tag{Key: key, Value: val})
	}
	return def, tags, nil
}

func validTagValue(key, val string) bool {
	switch key {
	case "member":
		return ValidMemberName(val)
	case "source":
		return val == "swim"
	case "state":
		return allowedOrchestratorStates[val]
	default:
		return false
	}
}

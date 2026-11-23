package metrics

import (
	"context"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hev/kit/internal/cluster"
	"github.com/hev/kit/internal/dispatch"
	"github.com/hev/kit/internal/secrets"
)

func intPtr(n int) *int { return &n }

func richFixtureSources() Sources {
	now := time.Now().UTC()
	return Sources{
		Self:           "oven",
		LocalVersion:   "0.7.0",
		ClusterEnabled: true,
		Members: func(context.Context) []cluster.Member {
			return []cluster.Member{
				{
					Facts: cluster.Facts{
						Name:    "oven",
						Version: "0.7.0",
						Activity: &cluster.Activity{
							HeadlessRuns:  intPtr(3),
							Orchestrators: intPtr(2),
							AgentsWorking: intPtr(4),
							AgentsIdle:    intPtr(1),
							At:            now,
						},
					},
					State: "alive",
					Self:  true,
				},
				{
					Facts: cluster.Facts{
						Name:    "mini",
						Version: "0.6.9",
						Activity: &cluster.Activity{
							HeadlessRuns:  intPtr(1),
							Orchestrators: intPtr(0),
							// AgentsWorking and AgentsIdle nil (e.g., Windows / no tmux) -> must be omitted, never 0.
							At: now,
						},
					},
					State: "alive",
				},
				{
					Facts: cluster.Facts{
						Name:    "laptop",
						Version: "0.7.0",
						Activity: &cluster.Activity{
							HeadlessRuns:  intPtr(9),
							Orchestrators: intPtr(9),
							AgentsWorking: intPtr(9),
							AgentsIdle:    intPtr(9),
							At:            now,
						},
					},
					State: "dead",
				},
				{
					// Adversarial member name containing path/tag delimiter must be dropped.
					Facts: cluster.Facts{Name: "/Users/alice/secret:token", Version: "0.7.0"},
					State: "alive",
				},
			}
		},
		SecretsStatus: func() *secrets.Status {
			return &secrets.Status{
				Budget: secrets.BudgetStatus{
					Hourly:     40,
					UsedHour:   12,
					Daily:      200,
					UsedDay:    50,
					LastCaller: "op://vault/secret-item/api_key",
				},
				ReadLog: "/home/alice/.hev/secrets/op-reads.log",
			}
		},
		Orchestrators: func() ([]dispatch.OrchestratorState, error) {
			return []dispatch.OrchestratorState{
				{Name: "secret-repo-orch", State: "running", Goal: "deploy sk_live_SECRET123", Error: "/Users/alice/repo"},
				{Name: "wait-1", State: "waiting"},
				{Name: "wait-idle", State: "idle"},
				{Name: "fail-1", State: "failed", Error: "token=op://vault/item/field"},
				{Name: "stall-1", State: "stalled", StallReason: "[needs you] secret prompt text"},
				{Name: "broken-1", State: "broken", Error: "open /home/alice/.config/hev/orchestrators/broken.md"},
				// Inactive/terminal and adversarial states must never appear.
				{Name: "paused-1", State: "paused"},
				{Name: "stopped-1", State: "stopped"},
				{Name: "done-1", State: "complete"},
				{Name: "evil-state", State: "/Users/alice/secret_state"},
			}, nil
		},
		CronFailing: func(context.Context) (int, error) {
			return 2, nil
		},
	}
}

func TestLoopbackDogStatsDAndPrometheusEmissionAndPrivacy(t *testing.T) {
	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udpConn.Close()

	receivedCh := make(chan string, 32)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			_ = udpConn.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, _, err := udpConn.ReadFrom(buf)
			if err != nil {
				return
			}
			receivedCh <- string(buf[:n])
		}
	}()

	cfg := Config{
		Enabled:        true,
		Interval:       "10s",
		DogStatsD:      udpConn.LocalAddr().String(),
		PrometheusAddr: "127.0.0.1:0",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	exp, err := Start(ctx, cfg, richFixtureSources(), nil)
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer exp.Stop()

	// Collect initial UDP batch(es) emitted immediately on Start.
	var udpLines []string
	deadline := time.After(3 * time.Second)
	for len(udpLines) < 20 {
		select {
		case pkt := <-receivedCh:
			if len(pkt) > MaxDatagramBytes {
				t.Fatalf("UDP packet exceeded MaxDatagramBytes: %d > %d", len(pkt), MaxDatagramBytes)
			}
			for _, line := range strings.Split(strings.TrimSpace(pkt), "\n") {
				if line != "" {
					udpLines = append(udpLines, line)
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for DogStatsD lines; got %d:\n%s", len(udpLines), strings.Join(udpLines, "\n"))
		}
	}

	gotUDP := map[string]bool{}
	for _, line := range udpLines {
		gotUDP[line] = true
	}

	wantUDPLines := []string{
		"hev.cluster.member_alive:1|g|#member:oven,source:swim",
		"hev.cluster.member_alive:1|g|#member:mini,source:swim",
		"hev.cluster.member_alive:0|g|#member:laptop,source:swim",
		"hev.cluster.version_skew:0|g|#member:oven,source:swim",
		"hev.cluster.version_skew:1|g|#member:mini,source:swim",
		"hev.cluster.version_skew:0|g|#member:laptop,source:swim",
		"hev.cluster.headless_runs:3|g|#member:oven,source:swim",
		"hev.cluster.orchestrators:2|g|#member:oven,source:swim",
		"hev.cluster.agents_working:4|g|#member:oven,source:swim",
		"hev.cluster.agents_idle:1|g|#member:oven,source:swim",
		"hev.cluster.headless_runs:1|g|#member:mini,source:swim",
		"hev.cluster.orchestrators:0|g|#member:mini,source:swim",
		"hev.secrets.op_used_hourly:12|g|#member:oven",
		"hev.secrets.op_hourly_pct:30|g|#member:oven",
		"hev.secrets.op_used_daily:50|g|#member:oven",
		"hev.secrets.op_daily_pct:25|g|#member:oven",
		"hev.orchestrator.by_state:1|g|#member:oven,state:running",
		"hev.orchestrator.by_state:2|g|#member:oven,state:waiting",
		"hev.orchestrator.by_state:1|g|#member:oven,state:failed",
		"hev.orchestrator.by_state:1|g|#member:oven,state:stalled",
		"hev.orchestrator.by_state:1|g|#member:oven,state:broken",
		"hev.cron.failing:2|g|#member:oven",
	}
	for _, want := range wantUDPLines {
		if !gotUDP[want] {
			t.Errorf("missing DogStatsD line %q\ngot:\n%s", want, strings.Join(udpLines, "\n"))
		}
	}

	// Dead member activity and nil activity counts must be omitted, never fabricated.
	for _, forbidden := range []string{
		"hev.cluster.headless_runs:9|g|#member:laptop,source:swim",
		"hev.cluster.agents_working:0|g|#member:mini,source:swim",
		"hev.cluster.agents_idle:0|g|#member:mini,source:swim",
	} {
		if gotUDP[forbidden] {
			t.Errorf("unexpected DogStatsD line emitted: %q", forbidden)
		}
	}

	// Scrape Prometheus /metrics endpoint.
	promAddr := exp.BoundPrometheusAddr()
	if promAddr == "" {
		t.Fatal("BoundPrometheusAddr() is empty")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + promAddr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read /metrics body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Fatalf("Content-Type=%q, want text/plain; version=0.0.4", ct)
	}
	promBody := string(bodyBytes)

	wantPromLines := []string{
		"# TYPE hev_cluster_member_alive gauge",
		`hev_cluster_member_alive{member="oven",source="swim"} 1`,
		`hev_cluster_member_alive{member="mini",source="swim"} 1`,
		`hev_cluster_member_alive{member="laptop",source="swim"} 0`,
		"# TYPE hev_cluster_version_skew gauge",
		`hev_cluster_version_skew{member="oven",source="swim"} 0`,
		`hev_cluster_version_skew{member="mini",source="swim"} 1`,
		`hev_cluster_version_skew{member="laptop",source="swim"} 0`,
		"# TYPE hev_cluster_headless_runs gauge",
		`hev_cluster_headless_runs{member="oven",source="swim"} 3`,
		`hev_cluster_headless_runs{member="mini",source="swim"} 1`,
		"# TYPE hev_cluster_orchestrators gauge",
		`hev_cluster_orchestrators{member="oven",source="swim"} 2`,
		`hev_cluster_orchestrators{member="mini",source="swim"} 0`,
		"# TYPE hev_cluster_agents_working gauge",
		`hev_cluster_agents_working{member="oven",source="swim"} 4`,
		"# TYPE hev_cluster_agents_idle gauge",
		`hev_cluster_agents_idle{member="oven",source="swim"} 1`,
		"# TYPE hev_secrets_op_used_hourly gauge",
		`hev_secrets_op_used_hourly{member="oven"} 12`,
		"# TYPE hev_secrets_op_hourly_pct gauge",
		`hev_secrets_op_hourly_pct{member="oven"} 30`,
		"# TYPE hev_secrets_op_used_daily gauge",
		`hev_secrets_op_used_daily{member="oven"} 50`,
		"# TYPE hev_secrets_op_daily_pct gauge",
		`hev_secrets_op_daily_pct{member="oven"} 25`,
		"# TYPE hev_orchestrator_by_state gauge",
		`hev_orchestrator_by_state{member="oven",state="running"} 1`,
		`hev_orchestrator_by_state{member="oven",state="waiting"} 2`,
		`hev_orchestrator_by_state{member="oven",state="failed"} 1`,
		`hev_orchestrator_by_state{member="oven",state="stalled"} 1`,
		`hev_orchestrator_by_state{member="oven",state="broken"} 1`,
		"# TYPE hev_cron_failing gauge",
		`hev_cron_failing{member="oven"} 2`,
	}
	for _, want := range wantPromLines {
		if !strings.Contains(promBody, want) {
			t.Errorf("missing Prometheus line %q\ngot:\n%s", want, promBody)
		}
	}

	// Privacy assertion: NO secrets, paths, repo names, prompts, or non-allowlisted states may appear in either payload.
	combinedUDP := strings.Join(udpLines, "\n")
	var promDataLines []string
	for _, line := range strings.Split(promBody, "\n") {
		if line != "" && !strings.HasPrefix(line, "#") {
			promDataLines = append(promDataLines, line)
		}
	}
	promData := strings.Join(promDataLines, "\n")
	forbiddenSubstrings := []string{
		"sk_live_",
		"op://",
		"/Users/",
		"/home/",
		"secret-repo-orch",
		"secret prompt text",
		"op-reads.log",
		"paused",
		"stopped",
		"complete",
		"evil-state",
		"alice",
	}
	for _, bad := range forbiddenSubstrings {
		if strings.Contains(combinedUDP, bad) {
			t.Errorf("DogStatsD payload leaked forbidden substring %q:\n%s", bad, combinedUDP)
		}
		if strings.Contains(promData, bad) {
			t.Errorf("Prometheus metric lines leaked forbidden substring %q:\n%s", bad, promData)
		}
	}

	// HTTP method and path guards.
	headResp, err := client.Head("http://" + promAddr + "/metrics")
	if err != nil {
		t.Fatalf("HEAD /metrics: %v", err)
	}
	headResp.Body.Close()
	if headResp.StatusCode != http.StatusOK {
		t.Fatalf("HEAD /metrics status=%d, want 200", headResp.StatusCode)
	}
	postResp, err := client.Post("http://"+promAddr+"/metrics", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("POST /metrics: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /metrics status=%d, want 405", postResp.StatusCode)
	}
	notFoundResp, err := client.Get("http://" + promAddr + "/secret")
	if err != nil {
		t.Fatalf("GET /secret: %v", err)
	}
	notFoundResp.Body.Close()
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /secret status=%d, want 404", notFoundResp.StatusCode)
	}

	// Graceful shutdown stops Prometheus listener cleanly.
	exp.Stop()
	if _, err := client.Get("http://" + promAddr + "/metrics"); err == nil {
		t.Fatal("Prometheus endpoint still reachable after Stop()")
	}
	_ = udpConn.Close()
	<-readDone
}

func TestAllowlistRefusesUnallowlistedMetricsTagsAndValues(t *testing.T) {
	badSamples := []Sample{
		{Name: "hev.secret.token", Tags: map[string]string{"member": "oven"}, Value: 1},
		{Name: MetricCronFailing, Tags: map[string]string{"member": "oven", "repo": "kit"}, Value: 1},
		{Name: MetricCronFailing, Tags: map[string]string{}, Value: 1},
		{Name: MetricClusterMemberAlive, Tags: map[string]string{"member": "oven", "source": "ssh"}, Value: 1},
		{Name: MetricOrchestratorByState, Tags: map[string]string{"member": "oven", "state": "paused"}, Value: 1},
		{Name: MetricOrchestratorByState, Tags: map[string]string{"member": "oven", "state": "/etc/passwd"}, Value: 1},
		{Name: MetricCronFailing, Tags: map[string]string{"member": "/home/alice"}, Value: 1},
		{Name: MetricCronFailing, Tags: map[string]string{"member": "oven,secret:123"}, Value: 1},
		{Name: MetricCronFailing, Tags: map[string]string{"member": "oven"}, Value: math.NaN()},
		{Name: MetricCronFailing, Tags: map[string]string{"member": "oven"}, Value: math.Inf(1)},
	}
	for i, s := range badSamples {
		if _, _, err := ValidateSample(s); err == nil {
			t.Errorf("case %d (%+v): ValidateSample unexpectedly accepted sample", i, s)
		}
	}
	if lines := FormatDogStatsD(badSamples); len(lines) != 0 {
		t.Fatalf("FormatDogStatsD emitted unallowlisted samples: %v", lines)
	}
	if prom := FormatPrometheus(badSamples); prom != "" {
		t.Fatalf("FormatPrometheus emitted unallowlisted samples:\n%s", prom)
	}
}

func TestSendDogStatsDRefusesNonPrivateDestination(t *testing.T) {
	s := []Sample{{Name: MetricCronFailing, Tags: map[string]string{"member": "oven"}, Value: 0}}
	for _, addr := range []string{"8.8.8.8:8125", "0.0.0.0:8125", "192.168.1.5:8125", "localhost:8125"} {
		if err := SendDogStatsD(addr, s); err == nil {
			t.Errorf("SendDogStatsD(%q) succeeded, want refusal", addr)
		}
	}
}

func TestBatchDogStatsDBoundsDatagramSize(t *testing.T) {
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, "hev.cluster.member_alive:1|g|#member:node-name-example,source:swim")
	}
	pkts := BatchDogStatsD(lines, 256)
	if len(pkts) < 2 {
		t.Fatalf("expected multiple batches, got %d", len(pkts))
	}
	totalLines := 0
	for i, pkt := range pkts {
		if len(pkt) > 256 {
			t.Fatalf("packet %d exceeded limit: %d > 256", i, len(pkt))
		}
		totalLines += len(strings.Split(string(pkt), "\n"))
	}
	if totalLines != len(lines) {
		t.Fatalf("lost lines during batching: got %d, want %d", totalLines, len(lines))
	}
}

func TestPlacementWitnessElectionAndFailover(t *testing.T) {
	members := []cluster.Member{
		{Facts: cluster.Facts{Name: "oven", Version: "0.7.0"}, State: "alive"},
		{Facts: cluster.Facts{Name: "mini", Version: "0.7.0"}, State: "alive"},
		{Facts: cluster.Facts{Name: "laptop", Version: "0.7.0"}, State: "alive"},
	}
	placement := []string{"oven", "mini"}

	ovenWins := EmitClusterAllowed(placement, "oven", true, members)
	miniWins := EmitClusterAllowed(placement, "mini", true, members)
	laptopWins := EmitClusterAllowed(placement, "laptop", true, members)

	if ovenWins == miniWins {
		t.Fatalf("expected exactly one witness between oven (%v) and mini (%v)", ovenWins, miniWins)
	}
	if laptopWins {
		t.Fatal("member outside placement should not emit cluster gauges")
	}

	winner, runnerUp := "oven", "mini"
	if miniWins {
		winner, runnerUp = "mini", "oven"
	}

	// Verify Collect emits hev.cluster.* on winner and only host-local gauges on runner-up.
	makeSrc := func(self string, ms []cluster.Member) Sources {
		return Sources{
			Self:           self,
			LocalVersion:   "0.7.0",
			ClusterEnabled: true,
			Members:        func(context.Context) []cluster.Member { return ms },
			CronFailing:    func(context.Context) (int, error) { return 1, nil },
		}
	}
	cfg := Config{Enabled: true, Placement: placement}

	winnerSamples := Collect(context.Background(), cfg, makeSrc(winner, members))
	loserSamples := Collect(context.Background(), cfg, makeSrc(runnerUp, members))

	hasCluster := func(samples []Sample) bool {
		for _, s := range samples {
			if strings.HasPrefix(s.Name, "hev.cluster.") {
				return true
			}
		}
		return false
	}
	hasLocal := func(samples []Sample) bool {
		for _, s := range samples {
			if s.Name == MetricCronFailing {
				return true
			}
		}
		return false
	}

	if !hasCluster(winnerSamples) || !hasLocal(winnerSamples) {
		t.Fatalf("winner should emit both cluster and local gauges: %+v", winnerSamples)
	}
	if hasCluster(loserSamples) {
		t.Fatalf("non-winner must not emit cluster-wide gauges: %+v", loserSamples)
	}
	if !hasLocal(loserSamples) {
		t.Fatalf("non-winner must still emit host-local gauges: %+v", loserSamples)
	}

	// Failover: mark winner dead -> runner-up must take over cluster-wide gauges.
	failedOver := make([]cluster.Member, len(members))
	copy(failedOver, members)
	for i := range failedOver {
		if failedOver[i].Name == winner {
			failedOver[i].State = "dead"
		}
	}
	if !EmitClusterAllowed(placement, runnerUp, true, failedOver) {
		t.Fatalf("runner-up %q failed to take over after witness %q died", runnerUp, winner)
	}

	// Without cluster enabled: multi-host placement fails closed; singleton matching self succeeds.
	if EmitClusterAllowed([]string{"oven", "mini"}, "oven", false, nil) {
		t.Fatal("multi-host placement must fail closed when cluster is disabled")
	}
	if !EmitClusterAllowed([]string{"oven"}, "oven", false, nil) {
		t.Fatal("singleton placement matching self should be allowed without cluster")
	}
	if EmitClusterAllowed([]string{"mini"}, "oven", false, nil) {
		t.Fatal("singleton placement for another host must be refused")
	}
}

func TestDisabledConfigStartsNothing(t *testing.T) {
	exp, err := Start(context.Background(), Config{
		Enabled:        false,
		PrometheusAddr: "127.0.0.1:0",
		DogStatsD:      "127.0.0.1:8125",
	}, richFixtureSources(), nil)
	if err != nil {
		t.Fatalf("Start(disabled) error: %v", err)
	}
	defer exp.Stop()
	if exp.BoundPrometheusAddr() != "" {
		t.Fatalf("disabled exporter unexpectedly bound %q", exp.BoundPrometheusAddr())
	}
}

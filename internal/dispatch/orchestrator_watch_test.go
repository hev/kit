package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsDeterministicWatch(t *testing.T) {
	orchHome(t)
	s := applyOrch(t)
	s.Agent = "off"
	s.Source = OrchestratorSource{}
	s.Authority = []string{"notify"}
	s.Reply = "worker/owner"
	s.NotifyRoute = "pull"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	st, err := orchestratorState(s.Name)
	if err != nil {
		t.Fatal(err)
	}
	state := "running"
	fail := false
	d := noticeDriver(t, nil)
	d.Gather = func(context.Context, OrchestratorSpec) ([]OrchestratorEvidence, error) {
		if fail {
			return nil, errors.New("unavailable")
		}
		return []OrchestratorEvidence{{Host: "worker", Row: Row{State: state}}}, nil
	}
	d.Session = func(context.Context, OrchestratorSpec, string, OrchestratorPacket) error {
		t.Fatal("watch launched model")
		return nil
	}
	for i, value := range []string{"running", "running", "complete", "running"} {
		state = value
		if err := RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
			t.Fatal(err)
		}
		want := []int{1, 1, 2, 3}[i]
		if got := len(noticesOf(t, s.Name, "watch")); got != want {
			t.Fatalf("step %d: notices %d want %d", i, got, want)
		}
	}
	// Persist/reload the observation, then prove unchanged facts stay quiet.
	if err := WriteJSON(filepath.Join(OrchestratorDir(s.Name), "state.json"), st); err != nil {
		t.Fatal(err)
	}
	st, err = orchestratorState(s.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err = RunOrchestratorRound(context.Background(), s, &st, d); err != nil {
		t.Fatal(err)
	}
	if got := len(noticesOf(t, s.Name, "watch")); got != 3 {
		t.Fatal(got)
	}
	fail = true
	if err = RunOrchestratorRound(context.Background(), s, &st, d); err == nil {
		t.Fatal("unknown evidence treated as facts")
	}
	if got := len(noticesOf(t, s.Name, "watch")); got != 3 {
		t.Fatal(got)
	}
	if _, err = os.Stat(filepath.Join(OrchestratorDir(s.Name), "rounds", "001", "packet.json")); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*OrchestratorSpec){func(s *OrchestratorSpec) { s.Reply = "" }, func(s *OrchestratorSpec) { s.Authority = []string{"dispatch", "notify"} }, func(s *OrchestratorSpec) { s.Idle = "run" }} {
		bad := s
		modify(&bad)
		if bad.Validate() == nil {
			t.Fatal("unsafe watch accepted")
		}
	}
}
